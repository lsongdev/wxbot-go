package wechatbot

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

// md5Sum calculates MD5 hash of data
func md5Sum(data []byte) []byte {
	h := md5.New()
	h.Write(data)
	return h.Sum(nil)
}

// calcCipherSize calculates the ciphertext size after AES-128-ECB encryption with PKCS7 padding.
func calcCipherSize(rawSize int64) int64 {
	return int64((int(rawSize) + 1) / 16 * 16)
}

// pkcs7Pad adds PKCS7 padding to plaintext for AES encryption.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// pkcs7Unpad removes PKCS7 padding from decrypted data.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := 0; i < padding; i++ {
		if data[len(data)-1-i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

// encryptAES128ECB encrypts plaintext using AES-128-ECB with PKCS7 padding.
// aesKey should be a 16-byte key or a 32-char hex string.
func encryptAES128ECB(plaintext []byte, aesKey string) ([]byte, error) {
	key, err := parseAESKey(aesKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	// ECB mode: encrypt each block independently
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return ciphertext, nil
}

// decryptAES128ECB decrypts ciphertext using AES-128-ECB.
// aesKey should be a 16-byte key or a 32-char hex string.
func decryptAES128ECB(ciphertext []byte, aesKey string) ([]byte, error) {
	key, err := parseAESKey(aesKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}
	plaintext := make([]byte, len(ciphertext))
	// ECB mode: decrypt each block independently
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(plaintext)
}

// parseAESKey parses an AES key from hex string or base64-encoded bytes.
// Supports both 16-byte raw key and 32-char hex string.
func parseAESKey(aesKey string) ([]byte, error) {
	// Try hex decode first (32-char hex string -> 16 bytes)
	if len(aesKey) == 32 {
		key, err := hex.DecodeString(aesKey)
		if err == nil && len(key) == 16 {
			return key, nil
		}
	}
	// Try base64 decode
	decoded, err := base64.StdEncoding.DecodeString(aesKey)
	if err != nil {
		return nil, fmt.Errorf("invalid AES key format: %w", err)
	}
	// If base64 decode gives 32 bytes, it might be base64(hex_string)
	// Try to decode the hex string
	if len(decoded) == 32 {
		key, err := hex.DecodeString(string(decoded))
		if err == nil && len(key) == 16 {
			return key, nil
		}
	}
	if len(decoded) == 16 {
		return decoded, nil
	}
	return nil, fmt.Errorf("invalid AES key length: expected 16 bytes, got %d", len(decoded))
}

func (c *WeChatBot) DownloadMedia(media *CDNMedia) ([]byte, error) {
	return c.DownloadFile(media.EncryptQueryParam, media.AESKey)
}

// DownloadFile downloads and decrypts a file from the CDN.
// encryptQueryParam is the encrypted query parameter from CDNMedia.
// aesKey is the AES key for decryption (can be hex string or base64-encoded).
func (c *WeChatBot) DownloadFile(encryptQueryParam string, aesKey string) ([]byte, error) {
	// encryptQueryParam may already be base64-encoded, try using it directly first
	// If it's not base64, encode it
	var encodedParam string
	if _, err := base64.StdEncoding.DecodeString(encryptQueryParam); err == nil {
		// Already valid base64, use as-is (but need to URL-encode for safe URL usage)
		encodedParam = encryptQueryParam
	} else {
		// Not base64, encode it
		encodedParam = base64.URLEncoding.EncodeToString([]byte(encryptQueryParam))
	}
	url := fmt.Sprintf("%s/download?encrypted_query_param=%s", c.CDNBaseURL, encodedParam)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: failed to download file", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	data, err := decryptAES128ECB(body, aesKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt file: %w", err)
	}
	return data, nil
}

// UploadFile uploads a file to the CDN and returns the upload result.
// mediaType: 1=IMAGE, 2=VIDEO, 3=FILE, 4=VOICE
// toUserID: the target user ID
// noNeedThumb: if true, skip thumbnail upload
func (c *WeChatBot) UploadFile(mediaType UploadMediaType, toUserID string, fileName string, fileData []byte, noNeedThumb bool) (*UploadFileResult, error) {
	// Generate random filekey and aeskey
	fileKey := make([]byte, 16)
	if _, err := rand.Read(fileKey); err != nil {
		return nil, fmt.Errorf("generate filekey: %w", err)
	}
	aesKey := make([]byte, 16)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("generate aeskey: %w", err)
	}
	aesKeyHex := hex.EncodeToString(aesKey)

	rawSize := int64(len(fileData))
	fileSize := calcCipherSize(rawSize)

	// Calculate MD5
	md5Hash := fmt.Sprintf("%x", md5Sum(fileData))

	// Step 1: Get upload URL
	uploadReq := &GetUploadURLReq{
		ToUserID:    toUserID,
		BaseInfo:    c.buildBaseInfo(),
		FileKey:     hex.EncodeToString(fileKey),
		AESKey:      aesKeyHex,
		MediaType:   int(mediaType),
		RawSize:     rawSize,
		FileSize:    fileSize,
		RawFileMD5:  md5Hash,
		NoNeedThumb: noNeedThumb,
	}
	uploadResp, err := c.GetUploadURL(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("getuploadurl: %w", err)
	}
	if uploadResp.UploadParam == "" {
		return nil, fmt.Errorf("empty upload_param from server")
	}

	// Step 2: Encrypt file
	ciphertext, err := encryptAES128ECB(fileData, aesKeyHex)
	if err != nil {
		return nil, fmt.Errorf("encrypt file: %w", err)
	}

	// Step 3: Upload to CDN
	// upload_param from server is already base64-encoded encrypted_query_param
	encryptedQueryParam := uploadResp.UploadParam

	// Don't double-encode - upload_param is already properly encoded
	url := fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s",
		c.CDNBaseURL,
		encryptedQueryParam,
		hex.EncodeToString(fileKey))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(ciphertext))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: failed to upload file", resp.StatusCode)
	}

	// Get x-encrypted-param from response header
	finalEncryptParam := resp.Header.Get("x-encrypted-param")
	if finalEncryptParam == "" {
		// Fallback to request param if header not present
		finalEncryptParam = encryptedQueryParam
	}
	return &UploadFileResult{
		CDNMedia: &CDNMedia{
			EncryptQueryParam: finalEncryptParam,
			AESKey:            base64.StdEncoding.EncodeToString([]byte(aesKeyHex)), // Format B: base64(hex string)
			EncryptType:       1,
		},
		FileSize: fileSize,
		RawSize:  rawSize,
	}, nil
}
