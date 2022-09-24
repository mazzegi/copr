package copr

import (
	"encoding/json"
	"os"
	"path/filepath"

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

func LoadUnits(dir string) (*Units, error) {
	adir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "abs-dir %q", dir)
	}
	us := &Units{
		dir: adir,
	}
	err = us.Load()
	if err != nil {
		return nil, errors.Wrap(err, "load units")
	}
	return us, nil
}

type Units struct {
	dir   string
	units []Unit
}

func (us *Units) Load() error {
	fis, err := os.ReadDir(us.dir)
	if err != nil {
		return errors.Wrapf(err, "read-dir %q", us.dir)
	}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}

		//
		unitFile := filepath.Join(us.dir, fi.Name(), "copr.unit.json")
		if _, err := os.Stat(unitFile); err != nil {
			continue
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
