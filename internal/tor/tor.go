package tor

//go:generate go-bindata -nometadata -nocompress -tags internalTor -o bindata.go -pkg tor ../../third_party/tor/tor

import (
	"io"
	"log"
	"os"
	"os/exec"
)

func runExecutable(exe string, args []string, logpath string) (*os.Process, error) {
	version, err := versionFromExe(exe)
	if err != nil {
		return nil, err
	}
	log.Printf("Detected %s\n", version)

	cmd := exec.Command(exe)
	cmd.Env = os.Environ()
	cmd.Args = append([]string{"procname"}, args...)

	if logpath != "" {
		logfile, err := os.OpenFile(logpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			log.Printf("Unable to open logfile \"%s\"\n%s\n", logpath, err.Error())
		} else {
			logWriter := io.Writer(logfile)
			cmd.Stdout = logWriter
			cmd.Stderr = logWriter
		}
	}

	log.Println("Starting Tor...")
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd.Process, nil
}

func versionFromExe(exe string) (string, error) {
	return runExeWithArgs(exe, "--version")
}

func pwHashFromExe(exe, pw string) (string, error) {
	return runExeWithArgs(exe, "--hash-password", pw)
}

func runExeWithArgs(exe string, args ...string) (o string, err error) {
	var r []byte
	r, err = exec.Command(exe, args...).Output()
	if err != nil {
		return "", err
	}

	o = string(r)
	o = o[:len(o)-1]

	return o, nil
}
