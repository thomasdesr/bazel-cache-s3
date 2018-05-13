package main

import (
	"context"
	"io"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

// S3Manager is a wrapper around S3 Upload & Download Managers (fancy wrappers around multipart API endpoints) that expose a groupcache & http put friendly API
type S3Manager struct {
	s3c *s3.S3
	up  *s3manager.Uploader
	dl  *s3manager.Downloader

	bucket string
}

// NewS3Manager defines which bucket will be used for data storage & initalizes the S3 managers
func NewS3Manager(s3c *s3.S3, bucket string) *S3Manager {
	return &S3Manager{
		s3c: s3c,
		up:  s3manager.NewUploaderWithClient(s3c),
		dl:  s3manager.NewDownloaderWithClient(s3c),

		bucket: bucket,
	}
}

func bestEffortGetSize(s3c *s3.S3, bucket, key string) int64 {
	r, _ := s3c.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if r != nil && r.ContentLength != nil {
		return *r.ContentLength
	}
	return 0
}

// Getter implements the groupcache.Getter interface
func (s *S3Manager) Getter(groupCacheContext groupcache.Context, key string, dest groupcache.Sink) error {
	log.Printf("Hydration request called, pulling %q from S3", key)

	var ctx = groupCacheContext.(context.Context)

	size := bestEffortGetSize(s.s3c, s.bucket, key)
	buf := aws.NewWriteAtBuffer(make([]byte, size))

	_, err := s.dl.DownloadWithContext(ctx, buf, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Println("Hydration failure:", err)
		return errors.Wrap(err, "failed to download object")
	}

	return errors.Wrap(
		dest.SetBytes(buf.Bytes()),
		"failed hydration when setting bytes on the groupcache sync",
	)
}

// PutReader passes the provided reader to an S3 Upload manager and tells it to upload the contents of that reader to the also given key
func (s *S3Manager) PutReader(ctx context.Context, key string, r io.Reader) error {
	_, err := s.up.UploadWithContext(
		ctx,
		&s3manager.UploadInput{
			Bucket: bucket,
			Key:    aws.String(key),
			Body:   r,
		},
	)

	return err
}
