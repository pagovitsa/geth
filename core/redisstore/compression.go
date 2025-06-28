package redisstore

import (
	"bytes"
	"compress/zlib"
	"io"
)

var config *Config

func SetConfig(cfg *Config) {
	config = cfg
}

func Compress(data []byte) ([]byte, error) {
	if config != nil && !config.CompressEnabled {
		return data, nil
	}
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	w.Close()
	return b.Bytes(), nil
}

func Decompress(data []byte) ([]byte, error) {
	if config != nil && !config.CompressEnabled {
		return data, nil
	}
	b := bytes.NewReader(data)
	r, err := zlib.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func CompressRatio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 1.0
	}
	return float64(len(compressed)) / float64(len(original))
}
