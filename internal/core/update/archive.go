package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
)

func unpack(data []byte) (binary []byte, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, gz.Close()) }()
	reader := tar.NewReader(io.LimitReader(gz, binaryLimit+checksumLimit))
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, nextErr
		}
		if header.Name != "kitbag" {
			continue
		}
		if !validEntry(header, binary) {
			return nil, errors.New("invalid executable archive entry")
		}
		binary, err = io.ReadAll(io.LimitReader(reader, binaryLimit+1))
		if err != nil {
			return nil, err
		}
	}
	// Consume the gzip footer too: a truncated gzip with complete tar data fails.
	if _, err = io.Copy(io.Discard, io.LimitReader(gz, checksumLimit)); err != nil {
		return nil, err
	}
	if len(binary) == 0 {
		return nil, errors.New("archive contains no kitbag executable")
	}
	return binary, nil
}

func validEntry(header *tar.Header, previous []byte) bool {
	return previous == nil && header.Typeflag == tar.TypeReg && header.Size > 0 && header.Size <= binaryLimit
}
