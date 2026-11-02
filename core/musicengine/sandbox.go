package musicengine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// InjectPolyfills populates a Goja runtime with essential browser & Node.js primitives
// required by LX Music user scripts: Buffer, TextEncoder/Decoder, crypto, fetch, setTimeout
func InjectPolyfills(vm *goja.Runtime) error {
	// 1. Global alias
	_ = vm.Set("global", vm.GlobalObject())
	_ = vm.Set("window", vm.GlobalObject())
	_ = vm.Set("self", vm.GlobalObject())

	// 2. Console
	console := vm.NewObject()
	_ = console.Set("log", func(call goja.FunctionCall) goja.Value {
		// Suppress verbose logs or trace if needed
		return goja.Undefined()
	})
	_ = console.Set("warn", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = console.Set("error", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = console.Set("info", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = vm.Set("console", console)

	// 3. Base64
	_ = vm.Set("btoa", func(call goja.FunctionCall) goja.Value {
		str := call.Argument(0).String()
		return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(str)))
	})
	_ = vm.Set("atob", func(call goja.FunctionCall) goja.Value {
		str := call.Argument(0).String()
		data, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return vm.ToValue(string(data))
	})

	// 4. TextEncoder & TextDecoder
	_ = vm.Set("TextEncoder", func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		_ = obj.Set("encode", func(fnCall goja.FunctionCall) goja.Value {
			str := fnCall.Argument(0).String()
			bytesVal := []byte(str)
			arr := make([]any, len(bytesVal))
			for i, b := range bytesVal {
				arr[i] = int(b)
			}
			return vm.ToValue(arr)
		})
		return obj
	})
	_ = vm.Set("TextDecoder", func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		_ = obj.Set("decode", func(fnCall goja.FunctionCall) goja.Value {
			arg := fnCall.Argument(0)
			if arg == nil || goja.IsUndefined(arg) {
				return vm.ToValue("")
			}
			var b []byte
			switch v := arg.Export().(type) {
			case []byte:
				b = v
			case []any:
				b = make([]byte, len(v))
				for i, item := range v {
					if num, ok := item.(int64); ok {
						b[i] = byte(num)
					}
				}
			case string:
				b = []byte(v)
			}
			return vm.ToValue(string(b))
		})
		return obj
	})

	// 5. Crypto polyfills (lx.utils.crypto & native crypto)
	injectCrypto(vm)

	// 6. HTTP Fetch
	injectFetch(vm)

	// 7. Timers
	injectTimers(vm)

	return nil
}

func injectCrypto(vm *goja.Runtime) {
	cryptoObj := vm.NewObject()

	// MD5
	_ = cryptoObj.Set("md5", func(call goja.FunctionCall) goja.Value {
		input := call.Argument(0).String()
		h := md5.Sum([]byte(input))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	// SHA1
	_ = cryptoObj.Set("sha1", func(call goja.FunctionCall) goja.Value {
		input := call.Argument(0).String()
		h := sha1.Sum([]byte(input))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	// SHA256
	_ = cryptoObj.Set("sha256", func(call goja.FunctionCall) goja.Value {
		input := call.Argument(0).String()
		h := sha256.Sum256([]byte(input))
		return vm.ToValue(hex.EncodeToString(h[:]))
	})

	// AES
	aesObj := vm.NewObject()
	_ = aesObj.Set("encrypt", func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).String()
		key := call.Argument(1).String()
		iv := call.Argument(2).String()
		mode := strings.ToLower(call.Argument(3).String()) // "cbc", "ecb", "gcm"

		encrypted, err := aesEncrypt([]byte(data), []byte(key), []byte(iv), mode)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString(encrypted))
	})
	_ = aesObj.Set("decrypt", func(call goja.FunctionCall) goja.Value {
		dataBase64 := call.Argument(0).String()
		key := call.Argument(1).String()
		iv := call.Argument(2).String()
		mode := strings.ToLower(call.Argument(3).String())

		rawBytes, err := base64.StdEncoding.DecodeString(dataBase64)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		decrypted, err := aesDecrypt(rawBytes, []byte(key), []byte(iv), mode)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return vm.ToValue(string(decrypted))
	})
	_ = cryptoObj.Set("aes", aesObj)

	// RSA
	rsaObj := vm.NewObject()
	_ = rsaObj.Set("encrypt", func(call goja.FunctionCall) goja.Value {
		data := call.Argument(0).String()
		pubKeyPEM := call.Argument(1).String()
		encrypted, err := rsaEncrypt([]byte(data), pubKeyPEM)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString(encrypted))
	})
	_ = cryptoObj.Set("rsa", rsaObj)

	_ = vm.Set("crypto", cryptoObj)

	// Also expose on lx.utils.crypto if required by LX scripts
	lxObj := vm.NewObject()
	utilsObj := vm.NewObject()
	_ = utilsObj.Set("crypto", cryptoObj)
	_ = lxObj.Set("utils", utilsObj)
	_ = vm.Set("lx", lxObj)
}

func injectFetch(vm *goja.Runtime) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	fetchFn := func(call goja.FunctionCall) goja.Value {
		rawURL := call.Argument(0).String()
		opts := call.Argument(1)

		method := http.MethodGet
		headers := make(map[string]string)
		var bodyReader io.Reader

		if opts != nil && !goja.IsUndefined(opts) {
			optObj := opts.ToObject(vm)
			if m := optObj.Get("method"); m != nil && !goja.IsUndefined(m) {
				method = strings.ToUpper(m.String())
			}
			if h := optObj.Get("headers"); h != nil && !goja.IsUndefined(h) {
				if hMap, ok := h.Export().(map[string]any); ok {
					for k, v := range hMap {
						headers[k] = fmt.Sprint(v)
					}
				}
			}
			if b := optObj.Get("body"); b != nil && !goja.IsUndefined(b) {
				bodyReader = bytes.NewBufferString(b.String())
			}
		}

		promise, resolve, reject := vm.NewPromise()

		go func() {
			req, err := http.NewRequest(method, rawURL, bodyReader)
			if err != nil {
				reject(vm.ToValue(err.Error()))
				return
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			}

			resp, err := client.Do(req)
			if err != nil {
				reject(vm.ToValue(err.Error()))
				return
			}
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				reject(vm.ToValue(err.Error()))
				return
			}

			respObj := vm.NewObject()
			_ = respObj.Set("status", resp.StatusCode)
			_ = respObj.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
			_ = respObj.Set("text", func(fnCall goja.FunctionCall) goja.Value {
				p, r, _ := vm.NewPromise()
				r(vm.ToValue(string(bodyBytes)))
				return vm.ToValue(p)
			})
			_ = respObj.Set("json", func(fnCall goja.FunctionCall) goja.Value {
				p, r, rej := vm.NewPromise()
				var parsed any
				if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
					rej(vm.ToValue(err.Error()))
				} else {
					r(vm.ToValue(parsed))
				}
				return vm.ToValue(p)
			})

			// Headers map
			respHeaders := vm.NewObject()
			for k, v := range resp.Header {
				if len(v) > 0 {
					_ = respHeaders.Set(strings.ToLower(k), v[0])
				}
			}
			_ = respObj.Set("headers", respHeaders)

			resolve(respObj)
		}()

		return vm.ToValue(promise)
	}

	_ = vm.Set("fetch", fetchFn)
	_ = vm.Set("httpFetch", fetchFn)
}

func injectTimers(vm *goja.Runtime) {
	var timerMu sync.Mutex
	timers := make(map[int64]*time.Timer)
	var timerIDSeq int64

	_ = vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return vm.ToValue(0)
		}
		delay := call.Argument(1).ToInteger()
		if delay < 0 {
			delay = 0
		}

		timerMu.Lock()
		timerIDSeq++
		id := timerIDSeq
		t := time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
			timerMu.Lock()
			delete(timers, id)
			timerMu.Unlock()
			_, _ = fn(goja.Undefined())
		})
		timers[id] = t
		timerMu.Unlock()

		return vm.ToValue(id)
	})

	_ = vm.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		id := call.Argument(0).ToInteger()
		timerMu.Lock()
		if t, ok := timers[id]; ok {
			t.Stop()
			delete(timers, id)
		}
		timerMu.Unlock()
		return goja.Undefined()
	})
}

// AES encryption helpers
func aesEncrypt(plainText, key, iv []byte, mode string) ([]byte, error) {
	block, err := aes.NewCipher(padKey(key, 16))
	if err != nil {
		return nil, err
	}

	switch mode {
	case "gcm":
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		nonce := padKey(iv, gcm.NonceSize())
		return gcm.Seal(nil, nonce, plainText, nil), nil
	case "ecb":
		plainText = pkcs7Pad(plainText, block.BlockSize())
		encrypted := make([]byte, len(plainText))
		for bs, be := 0, block.BlockSize(); bs < len(plainText); bs, be = bs+block.BlockSize(), be+block.BlockSize() {
			block.Encrypt(encrypted[bs:be], plainText[bs:be])
		}
		return encrypted, nil
	default: // "cbc"
		plainText = pkcs7Pad(plainText, block.BlockSize())
		iv = padKey(iv, block.BlockSize())
		ciphertext := make([]byte, len(plainText))
		mode := cipher.NewCBCEncrypter(block, iv)
		mode.CryptBlocks(ciphertext, plainText)
		return ciphertext, nil
	}
}

func aesDecrypt(cipherText, key, iv []byte, mode string) ([]byte, error) {
	block, err := aes.NewCipher(padKey(key, 16))
	if err != nil {
		return nil, err
	}

	switch mode {
	case "gcm":
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		nonce := padKey(iv, gcm.NonceSize())
		return gcm.Open(nil, nonce, cipherText, nil)
	case "ecb":
		if len(cipherText)%block.BlockSize() != 0 {
			return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
		}
		decrypted := make([]byte, len(cipherText))
		for bs, be := 0, block.BlockSize(); bs < len(cipherText); bs, be = bs+block.BlockSize(), be+block.BlockSize() {
			block.Decrypt(decrypted[bs:be], cipherText[bs:be])
		}
		return pkcs7Unpad(decrypted)
	default: // "cbc"
		if len(cipherText)%block.BlockSize() != 0 {
			return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
		}
		iv = padKey(iv, block.BlockSize())
		mode := cipher.NewCBCDecrypter(block, iv)
		decrypted := make([]byte, len(cipherText))
		mode.CryptBlocks(decrypted, cipherText)
		return pkcs7Unpad(decrypted)
	}
}

func padKey(key []byte, length int) []byte {
	if len(key) >= length {
		return key[:length]
	}
	padded := make([]byte, length)
	copy(padded, key)
	return padded
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("empty data")
	}
	unpadding := int(data[length-1])
	if unpadding > length || unpadding == 0 {
		return data, nil
	}
	return data[:(length - unpadding)], nil
}

func rsaEncrypt(data []byte, pubKeyPEM string) ([]byte, error) {
	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		pubKey, err2 := x509.ParsePKCS1PublicKey(block.Bytes)
		if err2 != nil {
			return nil, err
		}
		return rsa.EncryptPKCS1v15(rand.Reader, pubKey, data)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsa.EncryptPKCS1v15(rand.Reader, rsaPub, data)
}
