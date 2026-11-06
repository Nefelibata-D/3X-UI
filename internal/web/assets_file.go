package web

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
)

// Project: keep HTTP range seeking available after wrapping embedded assets for their modification time.
func newAssetsFile(file fs.File) (fs.File, error) {
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	wrapped := &wrapAssetsFile{file: file, info: info}
	if info.IsDir() {
		return wrapped, nil
	}
	content, err := io.ReadAll(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	wrapped.file = nil
	wrapped.reader = bytes.NewReader(content)
	return wrapped, nil
}

type wrapAssetsFile struct {
	file   fs.File
	reader *bytes.Reader
	info   fs.FileInfo
}

func (f *wrapAssetsFile) Read(p []byte) (int, error) {
	if f.reader != nil {
		return f.reader.Read(p)
	}
	return f.file.Read(p)
}

func (f *wrapAssetsFile) Seek(offset int64, whence int) (int64, error) {
	if f.reader == nil {
		return 0, errors.New("cannot seek an asset directory")
	}
	return f.reader.Seek(offset, whence)
}

func (f *wrapAssetsFile) Close() error {
	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

func (f *wrapAssetsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	dir, ok := f.file.(fs.ReadDirFile)
	if !ok {
		return nil, errors.New("asset directory does not support ReadDir")
	}
	return dir.ReadDir(n)
}

func (f *wrapAssetsFile) Stat() (fs.FileInfo, error) {
	return &wrapAssetsFileInfo{
		FileInfo: f.info,
	}, nil
}
