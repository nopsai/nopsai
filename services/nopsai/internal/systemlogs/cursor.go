package systemlogs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

type CursorCodec struct {
	key []byte
}

func NewCursorCodec(key []byte) *CursorCodec {
	keyCopy := append([]byte(nil), key...)
	return &CursorCodec{key: keyCopy}
}

func (c *CursorCodec) Encode(cursor Cursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	signature := c.sign(payload)
	token := append(append([]byte(nil), payload...), signature...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func (c *CursorCodec) Decode(token string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size {
		return Cursor{}, ErrCursorInvalid
	}
	payload := raw[:len(raw)-sha256.Size]
	signature := raw[len(raw)-sha256.Size:]
	if !hmac.Equal(signature, c.sign(payload)) {
		return Cursor{}, ErrCursorInvalid
	}
	var cursor Cursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.SourceID == "" || cursor.Sequence == 0 {
		return Cursor{}, ErrCursorInvalid
	}
	return cursor, nil
}

func (c *CursorCodec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
