package wechat

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidEvent = errors.New("微信事件签名或内容无效")

type componentEvent struct {
	AppID           string `xml:"AppId"`
	AuthorizerAppID string `xml:"AuthorizerAppid"`
	Type            string `xml:"InfoType"`
	Ticket          string `xml:"ComponentVerifyTicket"`
	CreatedAt       int64  `xml:"CreateTime"`
}

func decryptEvent(body []byte, cfg ComponentConfig, timestamp, nonce, signature string, now time.Time) (componentEvent, error) {
	var event componentEvent
	var envelope struct {
		Encrypted string `xml:"Encrypt"`
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || ts < now.Unix()-600 || ts > now.Unix()+600 || nonce == "" || len(nonce) > 256 ||
		xml.Unmarshal(body, &envelope) != nil || envelope.Encrypted == "" {
		return event, ErrInvalidEvent
	}
	parts := []string{cfg.EventToken, timestamp, nonce, envelope.Encrypted}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(signature)) != 1 {
		return event, ErrInvalidEvent
	}
	key, err := base64.RawStdEncoding.DecodeString(cfg.EncodingAESKey)
	data, dataErr := base64.StdEncoding.DecodeString(envelope.Encrypted)
	if err != nil || len(key) != 32 || dataErr != nil || len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return event, ErrInvalidEvent
	}
	block, _ := aes.NewCipher(key)
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(data, data)
	padding := int(data[len(data)-1])
	if padding < 1 || padding > 32 || padding > len(data) || !bytes.Equal(data[len(data)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return event, ErrInvalidEvent
	}
	data = data[:len(data)-padding]
	if len(data) < 20 {
		return event, ErrInvalidEvent
	}
	size := int(binary.BigEndian.Uint32(data[16:20]))
	if size > len(data)-20 || string(data[20+size:]) != cfg.AppID || xml.Unmarshal(data[20:20+size], &event) != nil ||
		event.AppID != cfg.AppID || event.CreatedAt <= 0 || event.CreatedAt > now.Unix()+600 {
		return event, ErrInvalidEvent
	}
	return event, nil
}
