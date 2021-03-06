package dtls

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

const cryptoGCMTagLength = 16

// State needed to handle encrypted input/output
type cryptoGCM struct {
	localGCM, remoteGCM         cipher.AEAD
	localWriteIV, remoteWriteIV []byte
}

func newCryptoGCM(localKey, localWriteIV, remoteKey, remoteWriteIV []byte) (*cryptoGCM, error) {
	localBlock, err := aes.NewCipher(localKey)
	if err != nil {
		return nil, err
	}
	localGCM, err := cipher.NewGCM(localBlock)
	if err != nil {
		return nil, err
	}

	remoteBlock, err := aes.NewCipher(remoteKey)
	if err != nil {
		return nil, err
	}
	remoteGCM, err := cipher.NewGCM(remoteBlock)
	if err != nil {
		return nil, err
	}

	return &cryptoGCM{
		localGCM:      localGCM,
		localWriteIV:  localWriteIV,
		remoteGCM:     remoteGCM,
		remoteWriteIV: remoteWriteIV,
	}, nil
}

func (c *cryptoGCM) encrypt(pkt *recordLayer, raw []byte) ([]byte, error) {
	payload := raw[recordLayerHeaderSize:]
	raw = raw[:recordLayerHeaderSize]

	nonce := append(append([]byte{}, c.localWriteIV[:4]...), make([]byte, 8)...)
	if _, err := rand.Read(nonce[4:]); err != nil {
		return nil, err
	}

	additionalData := generateAEADAdditionalData(&pkt.recordLayerHeader, len(payload))
	encryptedPayload := c.localGCM.Seal(nil, nonce, payload, additionalData)
	encryptedPayload = append(nonce[4:], encryptedPayload...)
	raw = append(raw, encryptedPayload...)

	// Update recordLayer size to include explicit nonce
	binary.BigEndian.PutUint16(raw[recordLayerHeaderSize-2:], uint16(len(raw)-recordLayerHeaderSize))
	return raw, nil

}

func (c *cryptoGCM) decrypt(in []byte) ([]byte, error) {
	var h recordLayerHeader
	err := h.Unmarshal(in)
	switch {
	case err != nil:
		return nil, err
	case h.contentType == contentTypeChangeCipherSpec:
		// Nothing to encrypt with ChangeCipherSpec
		return in, nil
	case len(in) <= (8 + recordLayerHeaderSize):
		return nil, errNotEnoughRoomForNonce
	}

	nonce := append(append([]byte{}, c.remoteWriteIV[:4]...), in[recordLayerHeaderSize:recordLayerHeaderSize+8]...)
	out := in[recordLayerHeaderSize+8:]

	additionalData := generateAEADAdditionalData(&h, len(out)-cryptoGCMTagLength)
	out, err = c.remoteGCM.Open(out[:0], nonce, out, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decryptPacket: %v", err)
	}
	return append(in[:recordLayerHeaderSize], out...), nil
}
