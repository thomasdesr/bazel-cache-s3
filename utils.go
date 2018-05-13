package main

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/pkg/errors"
)

func diskBufferBodies(tempDir string, next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		f, err := ioutil.TempFile(tempDir, "")
		if err != nil {
			e := "failed to create tempfile for upload buffering"
			log.Println(errors.Wrap(err, e))
			http.Error(rw, e, http.StatusInternalServerError)
		}
		defer func() {
			err := os.Remove(f.Name())
			if err != nil {
				log.Println(errors.Wrap(err, "failed to cleanup body buffer tempfile"))
			}
		}()

		if _, err := io.Copy(f, r.Body); err != nil {
			e := "failed to buffer PUT body correctly"
			log.Println(errors.Wrap(err, e))
			http.Error(rw, e, http.StatusInternalServerError)
		}

		if _, err := f.Seek(0, 0); err != nil {
			e := "failed to return file offset to the start of the file"
			log.Println(errors.Wrap(err, e))
			http.Error(rw, e, http.StatusInternalServerError)
		}

		r.Body = f

		next(rw, r)
	}
}
