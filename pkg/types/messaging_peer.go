package types

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/craumix/onionmsg/pkg/blobmngr"
	"github.com/craumix/onionmsg/pkg/sio/connection"
	"github.com/google/uuid"
)

const (
	queueTimeout = time.Second * 15
)

type MessagingPeer struct {
	RIdentity     RemoteIdentity       `json:"identity"`
	LastSyncState map[string]time.Time `json:"lastSync"`

	ctx         context.Context
	stop        context.CancelFunc
	skipTimeout context.CancelFunc

	Room *Room `json:"-"`
}

func NewMessagingPeer(rid RemoteIdentity) *MessagingPeer {
	return &MessagingPeer{
		RIdentity: rid,
	}
}

// RunMessageQueue creates a cancellable context for the MessagingPeer
// and starts a loop that will try to send queued messages every so often.
func (mp *MessagingPeer) RunMessageQueue(ctx context.Context, room *Room) {
	mp.Room = room

	mp.ctx, mp.stop = context.WithCancel(ctx)

	for {
		select {
		case <-mp.ctx.Done():
			log.Printf("Queue with %s in %s terminated!\n", mp.RIdentity.Fingerprint(), room.ID.String())
			return
		default:
			match := true
			for k, v := range mp.Room.SyncState {
				if t, ok := mp.LastSyncState[k]; !ok || t.Before(v) {
					match = false
				}
			}

			if match {
				break
			}

			err := mp.syncMsgs()
			if err != nil {
				//TODO Uncomment
				//log.Println(err)
			} else {
				mp.LastSyncState = CopySyncMap(mp.Room.SyncState)
			}
		}

		var skip context.Context
		skip, mp.skipTimeout = context.WithCancel(context.Background())

		select {
		case <-skip.Done(): //used to skip a single wait period
		case <-mp.ctx.Done(): //context cancelled
		case <-time.After(queueTimeout): //timeout
		}
	}
}

func (mp *MessagingPeer) BumpQueue() {
	if mp.skipTimeout != nil {
		mp.skipTimeout()
	}
}

func (mp *MessagingPeer) syncMsgs() error {
	if mp.Room == nil {
		return fmt.Errorf("Room not set")
	}

	conn, err := connection.GetConnFunc("tcp", mp.RIdentity.URL()+":"+strconv.Itoa(PubConvPort))
	if err != nil {
		return err
	}
	defer conn.Close()

	err = fingerprintChallenge(conn, mp.Room.Self)
	if err != nil {
		return err
	}

	conn.WriteBytes(mp.Room.ID[:], false)
	conn.Flush()

	err = expectResponse(conn, "auth_ok")
	if err != nil {
		return err
	}

	remoteSyncTimes := make(map[string]time.Time)
	err = conn.ReadStruct(&remoteSyncTimes, false)
	if err != nil {
		return err
	}

	msgsToSync := mp.findMessagesToSync(remoteSyncTimes)
	conn.WriteStruct(msgsToSync, true)
	conn.Flush()

	err = expectResponse(conn, "messages_ok")
	if err != nil {
		return err
	}

	blobIDs := blobIDsFromMessages(msgsToSync...)
	err = sendBlobs(conn, blobIDs)
	if err != nil {
		return err
	}

	err = expectResponse(conn, "sync_ok")
	if err != nil {
		return err
	}
	log.Println("transfer done")
	return nil
}

func sendBlobs(conn connection.ConnWrapper, ids []uuid.UUID) error {
	conn.WriteStruct(ids, false)
	conn.Flush()

	for _, id := range ids {
		stat, err := blobmngr.StatFromID(id)
		if err != nil {
			return err
		}

		blockCount := int(stat.Size() / blocksize)
		if stat.Size()%blocksize != 0 {
			blockCount++
		}

		_, err = conn.WriteInt(blockCount)
		if err != nil {
			return err
		}

		file, err := blobmngr.FileFromID(id)
		if err != nil {
			return err
		}
		defer file.Close()

		buf := make([]byte, blocksize)
		for c := 0; c < blockCount; c++ {
			n, err := file.Read(buf)
			if err != nil {
				return err
			}

			conn.WriteBytes(buf[:n], false)
			conn.Flush()

			err = expectResponse(conn, "block_ok")
			if err != nil {
				return err
			}
		}

		err = expectResponse(conn, "blob_ok")
		if err != nil {
			return err
		}

		log.Printf("Transfered Blob %s", id.String())
	}

	return nil
}

func blobIDsFromMessages(msgs ...Message) []uuid.UUID {
	ids := make([]uuid.UUID, 0)

	for _, msg := range msgs {
		if msg.ContainsBlob() {
			ids = append(ids, msg.Content.Meta.BlobUUID)
		}
	}

	return ids
}

func expectResponse(conn connection.ConnWrapper, expResp string) error {
	resp, err := conn.ReadString()
	if err != nil {
		return err
	} else if resp != expResp {
		return fmt.Errorf("received response \"%s\" wanted \"%s\"", resp, expResp)
	}

	return nil
}

func fingerprintChallenge(conn connection.ConnWrapper, id Identity) error {
	challenge, err := conn.ReadBytes(false)
	if err != nil {
		return err
	}

	conn.WriteString(id.Fingerprint())
	conn.WriteBytes(id.Sign(challenge), false)
	conn.Flush()

	return nil
}

func (mp *MessagingPeer) findMessagesToSync(remoteSyncTimes map[string]time.Time) []Message {
	msgs := make([]Message, 0)

	for _, msg := range mp.Room.Messages {
		if last, ok := remoteSyncTimes[msg.Meta.Sender]; !ok || msg.Meta.Time.After(last) {
			msgs = append(msgs, msg)
		}
	}

	return msgs
}
