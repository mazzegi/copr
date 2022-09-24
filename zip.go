package copr

import (
	"archive/zip"
	"io"

	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func addFilesToZip(w *zip.Writer, basePath, baseInZip string) error {
	files, err := os.ReadDir(basePath)
	if err != nil {
		return err
	}
	for _, file := range files {
		fullfilepath := filepath.Join(basePath, file.Name())
		if _, err := os.Stat(fullfilepath); os.IsNotExist(err) {
			// ensure the file exists. For example a symlink pointing to a non-existing location might be listed but not actually exist
			continue
		}
		if file.Type()&os.ModeSymlink != 0 {
			// ignore symlinks alltogether
			continue
		}

		if file.IsDir() {
			if err := addFilesToZip(w, fullfilepath, filepath.Join(baseInZip, file.Name())); err != nil {
				return errors.Wrapf(err, "add-files-to-zip %q", fullfilepath)
			}
			continue
		}
		if !file.Type().IsRegular() {
			continue
		}

		dat, err := os.ReadFile(fullfilepath)
		if err != nil {
			return err
		}
		f, err := w.Create(filepath.Join(baseInZip, file.Name()))
		if err != nil {
			return err
		}
		_, err = f.Write(dat)
		if err != nil {
			return err
		}
	}
	return nil
}

func ZipDir(w io.Writer, dir string) error {
	zw := zip.NewWriter(w)
	if err := addFilesToZip(zw, dir, ""); err != nil {
		return errors.Wrapf(err, "add-files-to-zip %q", dir)
	}
	if err := zw.Close(); err != nil {
		return errors.Wrap(err, "closing zipwriter")
	}
	return nil
}

func UnzipTo(zipfile string, dir string) error {
	archive, err := zip.OpenReader(zipfile)
	if err != nil {
		return errors.Wrapf(err, "zip-open-reader %q", zipfile)
	}
	defer archive.Close()

	for _, f := range archive.File {
		filePath := filepath.Join(dir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return errors.Wrapf(err, "mkdirall %q", filePath)
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return errors.Wrapf(err, "open-file %q", filePath)
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return errors.Wrapf(err, "open-archive-file %q", f.Name)
		}
		if _, err := io.Copy(dstFile, fileInArchive); err != nil {
			return errors.Wrapf(err, "copy %q", f.Name)
		}

		dstFile.Close()
		fileInArchive.Close()
	}
	return nil
}
