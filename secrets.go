package copr

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mazzegi/copr/secrets"
	"github.com/pkg/errors"
)

const (
	SecretFile = "copr.secrets"
)

var escapeTOMLReplacer = strings.NewReplacer(
	`\`, `\\`,
)

func NewSecrets(path string, pwd string) (*Secrets, error) {
	f, err := secrets.LoadFile(path, pwd)
	if err != nil {
		return nil, errors.Wrapf(err, "load secrets-file %q", path)
	}

	vals := map[string]string{}
	//_, err = toml.Decode(escapeTOMLReplacer.Replace(f.String()), &vals)
	_, err = toml.Decode(f.String(), &vals)
	if err != nil {
		return nil, errors.Wrap(err, "toml-decode")
	}

	return &Secrets{
		file:   f,
		values: vals,
	}, nil
}

type Secrets struct {
	file   *secrets.File
	values map[string]string
}

func (scs *Secrets) Find(key string) (string, bool) {
	v, ok := scs.values[key]
	return v, ok
}

func (scs *Secrets) Keys() []string {
	var ks []string
	for k := range scs.values {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (scs *Secrets) Expanded(s string) string {
	var oldnew []string
	for k, v := range scs.values {
		oldnew = append(oldnew, fmt.Sprintf("{%s}", k), v)
	}
	return strings.NewReplacer(oldnew...).Replace(s)
}

func (scs *Secrets) Save() error {
	buf := &bytes.Buffer{}
	err := toml.NewEncoder(buf).Encode(scs.values)
	if err != nil {
		return errors.Wrap(err, "toml-encode")
	}
	scs.file.Set(buf.Bytes())
	return scs.file.Save()
}

func (scs *Secrets) Delete(key string) {
	delete(scs.values, key)
}

func (scs *Secrets) Set(key, value string) {
	scs.values[key] = value
}
