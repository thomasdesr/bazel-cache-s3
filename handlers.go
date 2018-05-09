package main

import (
	"io"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

func bazelClientHandler(group *groupcache.Group, s3 *S3) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[1:]

		switch r.Method {
		case "HEAD", "GET":
			var b groupcache.ByteView
			err := group.Get(nil, key, groupcache.ByteViewSink(&b))
			if err := errors.Cause(err); err != nil {
				if awsErr, ok := err.(awserr.RequestFailure); ok && awsErr.StatusCode() == http.StatusNotFound {
					http.NotFound(rw, r)
					return
				}

				log.Println(errors.Wrap(err, "http get request failed"))
				http.Error(rw, "failed to retrieve key", http.StatusInternalServerError)
			}

			if _, err := io.Copy(rw, b.Reader()); err != nil {
				log.Println(errors.Wrap(err, "error sending http get reply"))
			}
		case "PUT":
			err := s3.PutReader(r.Context(), key, r.Body)

			if err != nil {
				log.Println(errors.Wrap(err, "http put request failed"))
				http.Error(rw, "put failed", http.StatusInternalServerError)
				return
			}
		}
	}
}
