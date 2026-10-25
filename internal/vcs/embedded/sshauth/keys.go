package sshauth

import (
	"bytes"
	"errors"
	"io/fs"

	"golang.org/x/crypto/ssh"
)

func parseKey(data, passphrase []byte) (ssh.Signer, error) {
	if passphrase == nil {
		return ssh.ParsePrivateKey(data)
	}
	return ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
}

func (a *Auth) fileSigner(path string) (ssh.Signer, error) {
	data, err := a.opts.FS.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, errors.New("cannot read SSH identity; check IdentityFile and permissions")
	}
	defer clear(data)
	signer, err := a.opts.ParseKey(data, nil)
	var encrypted *ssh.PassphraseMissingError
	if !errors.As(err, &encrypted) {
		if err != nil {
			return nil, errors.New("cannot parse SSH identity; check IdentityFile format")
		}
		return signer, nil
	}
	return a.decrypt(path, data)
}

func (a *Auth) decrypt(path string, data []byte) (ssh.Signer, error) {
	if signer := a.keyringSigner(path, data); signer != nil {
		return signer, nil
	}

	if !a.opts.Interactive || a.opts.Prompter == nil {
		return nil, fail(ErrLocked, "SSH identity requires a passphrase in non-interactive mode; unlock it in an SSH agent")
	}
	pass, store, err := a.opts.Prompter.Passphrase(a.ctx, a.Target.label(path))
	defer clear(pass)
	if err != nil {
		return nil, fail(ErrLocked, "SSH passphrase prompt cancelled; unlock the identity in an SSH agent")
	}
	signer, err := a.opts.ParseKey(data, pass)
	if err != nil {
		return nil, fail(ErrLocked, "SSH identity passphrase was rejected; unlock it in an SSH agent")
	}
	if store && a.opts.Keyring != nil {
		if err = a.opts.Keyring.Put(a.ctx, path, pass); err != nil {
			return nil, errors.New("cannot store SSH passphrase in keyring")
		}
	}
	return signer, nil
}

func (a *Auth) filterAgent(signers []ssh.Signer) []ssh.Signer {
	var allowed []ssh.Signer
	for _, signer := range signers {
		for _, path := range a.Target.Identities {
			if key := a.identityPublicKey(path); key != nil && bytes.Equal(key.Marshal(), signer.PublicKey().Marshal()) {
				allowed = append(allowed, signer)
				break
			}
		}
	}
	return allowed
}

func (a *Auth) identityPublicKey(path string) ssh.PublicKey {
	if data, err := a.opts.FS.ReadFile(path + ".pub"); err == nil {
		key, _, _, _, parseErr := ssh.ParseAuthorizedKey(data)
		if parseErr == nil {
			return key
		}
	}
	data, err := a.opts.FS.ReadFile(path)
	if err != nil {
		return nil
	}
	defer clear(data)
	signer, err := a.opts.ParseKey(data, nil)
	if err == nil {
		return signer.PublicKey()
	}
	var encrypted *ssh.PassphraseMissingError
	if errors.As(err, &encrypted) {
		return encrypted.PublicKey
	}
	return nil
}

func (a *Auth) keyringSigner(path string, data []byte) ssh.Signer {
	if a.opts.Keyring == nil {
		return nil
	}
	pass, err := a.opts.Keyring.Get(a.ctx, path)
	defer clear(pass)
	if err != nil {
		return nil
	}
	signer, err := a.opts.ParseKey(data, pass)
	if err != nil {
		return nil
	}
	return signer
}
