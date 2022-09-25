package copr

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// Unit represenst on Service/Program, considered to reside in one directory
type UnitConfig struct {
	//Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	Program         string   `json:"program"`
	Args            []string `json:"args,omitempty"`
	Env             []string `json:"env,omitempty"`
	RestartAfterSec int      `json:"restart-after-sec"`
}

type Unit struct {
	Dir    string
	Name   string
	Config UnitConfig
}

func LoadUnits(dir string, secs *Secrets) (*Units, error) {
	adir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "abs-dir %q", dir)
	}
	us := &Units{
		dir:     adir,
		secrets: secs,
	}
	os.MkdirAll(filepath.Join(adir, archiveDir), os.ModePerm)
	err = us.Load()
	if err != nil {
		return nil, errors.Wrap(err, "load units")
	}
	return us, nil
}

type Units struct {
	dir     string
	units   []Unit
	secrets *Secrets
}

const (
	archiveDir = ".archive"
)

func (us *Units) Load() error {
	fis, err := os.ReadDir(us.dir)
	if err != nil {
		return errors.Wrapf(err, "read-dir %q", us.dir)
	}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		if fi.Name() == archiveDir {
			continue
		}

		//
		unitFile := filepath.Join(us.dir, fi.Name(), "copr.unit.json")
		if _, err := os.Stat(unitFile); err != nil {
			continue
		}

		bs, err := os.ReadFile(unitFile)
		if err != nil {
			return errors.Wrapf(err, "read unit file %q", unitFile)
		}
		ebs := []byte(us.secrets.Expanded(string(bs)))
		var uc UnitConfig
		err = json.Unmarshal(ebs, &uc)
		if err != nil {
			return errors.Wrapf(err, "failed to json-decode unit file %q", unitFile)
		}

		us.units = append(us.units, Unit{
			Name:   fi.Name(),
			Dir:    filepath.Join(us.dir, fi.Name()),
			Config: uc,
		})
	}
	return nil
}

func (us *Units) loadUnit(unit string) (Unit, error) {
	unitFile := filepath.Join(us.dir, unit, "copr.unit.json")
	if _, err := os.Stat(unitFile); err != nil {
		return Unit{}, errors.Wrapf(err, "no unit file %q", unitFile)
	}

	f, err := os.Open(unitFile)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "failed to open unit file %q", unitFile)
	}
	defer f.Close()
	var uc UnitConfig
	err = json.NewDecoder(f).Decode(&uc)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "failed to json-decode unit file %q", unitFile)
	}
	return Unit{
		Name:   unit,
		Dir:    filepath.Join(us.dir, unit),
		Config: uc,
	}, nil
}

func (us *Units) SaveUnit(u Unit) error {
	unitFile := filepath.Join(u.Dir, "copr.unit.json")
	f, err := os.Create(unitFile)
	if err != nil {
		return errors.Wrapf(err, "create unitfile %q", unitFile)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(u.Config)
	if err != nil {
		return errors.Wrapf(err, "json-encode unitfile %q", unitFile)
	}
	return nil
}

func (us *Units) Create(unit string, dir string) (Unit, error) {
	newDir := filepath.Join(us.dir, unit)
	err := os.Rename(dir, newDir)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "rename %q -> %q", dir, newDir)
	}
	u, err := us.loadUnit(unit)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "load-unit %q", unit)
	}

	prg := filepath.Join(newDir, u.Config.Program)
	err = os.Chmod(prg, 0755)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "chmod program %q to 0755", prg)
	}

	us.units = append(us.units, u)

	return u, nil
}

func (us *Units) Update(unit string, dir string) (Unit, error) {
	//archive old unit dir
	unitDir := filepath.Join(us.dir, unit)
	archUnitFile := filepath.Join(us.dir, archiveDir, fmt.Sprintf("%s_%s_%03d.bak.zip", unit, time.Now().Format("20060102150405"), rand.Intn(1000)))
	archF, err := os.Create(archUnitFile)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "create archive in %q", archUnitFile)
	}
	defer archF.Close()
	err = ZipDir(archF, unitDir)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "create zip in %q", archUnitFile)
	}
	err = os.RemoveAll(unitDir)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "remove old unitdir %q", unitDir)
	}

	//
	err = os.Rename(dir, unitDir)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "rename %q -> %q", dir, unitDir)
	}
	u, err := us.loadUnit(unit)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "load-unit %q", unit)
	}

	prg := filepath.Join(unitDir, u.Config.Program)
	err = os.Chmod(prg, 0755)
	if err != nil {
		return Unit{}, errors.Wrapf(err, "chmod program %q to 0755", prg)
	}

	for i, u := range us.units {
		if u.Name == unit {
			us.units[i] = u
		}
	}
	return u, nil
}

func ValidateUnitDir(dir string) error {
	unitFile := filepath.Join(dir, "copr.unit.json")
	if _, err := os.Stat(unitFile); err != nil {
		return errors.Wrapf(err, "no unit file %q", unitFile)
	}
	f, err := os.Open(unitFile)
	if err != nil {
		return errors.Wrapf(err, "failed to open unit file %q", unitFile)
	}
	defer f.Close()
	var uc UnitConfig
	err = json.NewDecoder(f).Decode(&uc)
	if err != nil {
		return errors.Wrapf(err, "failed to json-decode unit file %q", unitFile)
	}
	return nil
}
