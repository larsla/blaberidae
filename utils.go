package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func downloadFile(filepath string, url string) error {

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func unzip(filepath, dest string) error {
	r, err := zip.OpenReader(filepath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		log.Println("Extracting file ", f.Name)
		fd, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dest + "/" + f.Name)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, fd)
		if err != nil {
			return err
		}
		fd.Close()
		out.Close()
	}

	return nil
}

// from https://gist.github.com/indraniel/1a91458984179ab4cf80
func unpackTarGz(srcFile string, dest string) error {

	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzf)
	// defer io.Copy(os.Stdout, tarReader)

	for true {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if strings.Contains(header.Name, "/") {
			err = os.MkdirAll(filepath.Dir(header.Name), 0755)
			if err != nil {
				return err
			}
		}

		name := fmt.Sprintf("%s/%s", dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			log.Println("Directory:", name)
			os.Mkdir(name, 0755)
		default:
			log.Printf("%v\n", header.Typeflag)
			log.Println("Unpacking file:", name)

			f, err := os.Create(name)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(f, tarReader)
			if err != nil {
				return err
			}

			err = os.Chmod(name, os.FileMode(header.Mode))
			if err != nil {
				log.Printf("Could not set mode %v on file %s", header.Mode, name)
			}
		}
	}
	return nil
}
