package types

import (
	"crypto/ed25519"
	"encoding/binary"
	"time"
)

const (
	MTYPE_TEXT = 0x00
	MTYPE_CMD  = 0x01
	MTYPE_BIN  = 0x02
)

type Message struct {
	Sender    string    `json:"sender"`
	Time      time.Time `json:"time"`
	Type      byte      `json:"type"`
	Content   []byte    `json:"content"`
	Signature []byte    `json:"signature"`
}

func (m *Message) Sign(priv ed25519.PrivateKey) {
	m.Signature = ed25519.Sign(priv, m.digestBytes())
}

func (m *Message) Verify(pub ed25519.PublicKey) bool {
	if m.Signature == nil {
		return false
	}

	return ed25519.Verify(pub, m.digestBytes(), m.Signature)
}

func (m *Message) digestBytes() []byte {
	d := []byte(m.Sender)
	d = append(d, int64ToBytes(m.Time.Unix())...)
	d = append(d, m.Type)
	d = append(d, m.Content...)

	return d
}

func int64ToBytes(i int64) []byte {
	bs := make([]byte, 8)
	binary.LittleEndian.PutUint64(bs, uint64(i))
	return bs
}
