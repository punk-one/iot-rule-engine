package rulesdb

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const connectorCipherPrefix = "enc:v1:"

func (s *SQLiteDB) initConnectorCipher() error {
	keyPath := os.Getenv("IOT_RULE_CONNECTOR_KEY_FILE")
	if keyPath == "" {
		keyPath = filepath.Join(filepath.Dir(s.path), "connector.key")
	}
	key, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM connectors WHERE resource LIKE 'enc:v1:%'").Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return errors.New("connector encryption key missing: restore the original key file")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		f, createErr := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr != nil {
			return fmt.Errorf("create connector key: %w", createErr)
		}
		_, err = f.Write(key)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return fmt.Errorf("read connector key: %w", err)
	}
	if len(key) != 32 {
		return errors.New("connector encryption key must contain exactly 32 bytes")
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("connector key file permissions must exclude group and other access")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	s.connectorCipher, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}
	rows, err := s.db.Query("SELECT id, resource FROM connectors")
	if err != nil {
		return err
	}
	type record struct{ id, data string }
	var legacy []record
	for rows.Next() {
		var r record
		if err = rows.Scan(&r.id, &r.data); err != nil {
			rows.Close()
			return err
		}
		if strings.HasPrefix(r.data, connectorCipherPrefix) {
			if _, err = s.decodeConnector(r.data); err != nil {
				rows.Close()
				return err
			}
		} else {
			legacy = append(legacy, r)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range legacy {
		encrypted, err := s.encodeConnector([]byte(r.data))
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE connectors SET resource = ? WHERE id = ?", encrypted, r.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteDB) encodeConnector(raw []byte) (string, error) {
	nonce := make([]byte, s.connectorCipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.connectorCipher.Seal(nonce, nonce, raw, []byte(connectorCipherPrefix))
	return connectorCipherPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *SQLiteDB) decodeConnector(raw string) ([]byte, error) {
	if !strings.HasPrefix(raw, connectorCipherPrefix) {
		return nil, errors.New("unencrypted connector record; reopen database to migrate")
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, connectorCipherPrefix))
	if err != nil || len(sealed) < s.connectorCipher.NonceSize() {
		return nil, errors.New("invalid encrypted connector record")
	}
	n := s.connectorCipher.NonceSize()
	data, err := s.connectorCipher.Open(nil, sealed[:n], sealed[n:], []byte(connectorCipherPrefix))
	if err != nil {
		return nil, errors.New("connector decryption failed: key mismatch or corrupt record")
	}
	return data, nil
}
