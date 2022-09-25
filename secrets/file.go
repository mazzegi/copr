package secrets

import (
	"os"

	"github.com/pkg/errors"
)

func cloneBytes(src []byte) []byte {
	cbs := make([]byte, len(src))
	copy(cbs, src)
	return cbs
}

func LoadFile(path string, pwd string) (*File, error) {
	if _, err := os.Stat(path); err != nil {
		// file doesn't exist
		return &File{
			path: path,
			pwd:  pwd,
			data: []byte{},
		}, nil
	}

	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "read-file %q", path)
	}

	dec, err := decrypt(bs, pwd)
	if err != nil {
		return nil, errors.Wrap(err, "decrpyt")
	}
	return &File{
		path: path,
		pwd:  pwd,
		data: dec,
	}, nil
}

type File struct {
	path string
	pwd  string
	data []byte
}

func (f *File) Data() []byte {
	return cloneBytes(f.data)
}

func (f *File) String() string {
	return string(f.Data())
}

func (f *File) Set(data []byte) {
	f.data = cloneBytes(data)
}

func (f *File) SetString(s string) {
	f.data = cloneBytes([]byte(s))
}

func (f *File) Save() error {
	enc, err := encrypt(f.data, f.pwd)
	if err != nil {
		return errors.Wrap(err, "encrypt")
	}
	err = os.WriteFile(f.path, enc, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "write-file %q", f.path)
	}
	return nil
}
