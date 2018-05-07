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

// S3Cache manages the relationship between groupcache and S3
type S3Cache struct {
	s3c *s3.S3
	up  *s3manager.Uploader
	dl  *s3manager.Downloader

	bucket string
}

func NewS3Cache(s3c *s3.S3, bucket string) *S3Cache {
	return &S3Cache{
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

// Get implements the groupcache.Getter interface
func (s *S3Cache) Get(groupCacheContext groupcache.Context, key string, dest groupcache.Sink) error {
	log.Printf("Hydration request called, pulling %q from S3", key)

	size := bestEffortGetSize(s.s3c, s.bucket, key)
	buf := aws.NewWriteAtBuffer(make([]byte, size))

	_, err := s.dl.Download(buf, &s3.GetObjectInput{
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

func (s *S3Cache) Put(ctx context.Context, key string, r io.Reader) error {
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
