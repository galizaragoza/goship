package remote

import (
	"bytes"
	"fmt"
	"net"

	"goship/internal/config"

	"golang.org/x/crypto/ssh"
)

func All(cfg *config.Config, cmd string) error {
	master := cfg.Master
	for j := range master.Subjects {
		fmt.Println(master.Creds) // DEBUG PURPOSE
		subject := master.Subjects[j]
		out, err := Exec(subject.Name, subject.IP.String(), subject.Port,
			master.Creds, cmd)
		if err != nil {
			return err
		}
		fmt.Println(out)
	}
	return nil
}

func Exec(user string, addr string, port string, keyPath string, cmd string) (string, error) {
	key, err := ssh.ParsePrivateKey([]byte(keyPath))
	if err != nil {
		return "", err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(key)},
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(addr, port), config)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var b bytes.Buffer

	session.Stdout = &b
	if err := session.Run("whoami"); err != nil {
		return "", nil
	}
	fmt.Println(b.String())
	return "", nil
}
