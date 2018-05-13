package main

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

func logCacheStats(group *groupcache.Group, interval time.Duration) {
	for t := time.Tick(interval); ; <-t {
		log.Printf("Stats | %+v", group.Stats)
		log.Printf("CacheStats:MainCache | %+v", group.CacheStats(groupcache.MainCache))
		log.Printf("CacheStats:HotCache | %+v", group.CacheStats(groupcache.HotCache))
	}
}

type cacheServer struct {
	s3m *S3Manager

	group *groupcache.Group
	gpool *groupcache.HTTPPool
}

// newCacheServer provides an HTTP server that implements a bazel cache endpoint. It uses an S3Manager to store cachable actions and objects into S3nd a groupcache pool to cache objects
func newCacheServer(s3m *S3Manager, self string, getter groupcache.Getter, updater Updater) *cacheServer {
	// Create group of cached objects
	group := groupcache.NewGroup(
		"bazelcache",
		2<<32,
		getter,
	)
	go logCacheStats(group, time.Second*15)

	// Find our peers
	pool := groupcache.NewHTTPPoolOpts(self, nil)
	go func() {
		if err := updater(pool); err != nil {
			log.Fatal(errors.Wrap(err, "updater failed"))
		}
	}()

	cs := &cacheServer{
		s3m: s3m,

		group: group,
		gpool: pool,
	}

	return cs
}

func (c *cacheServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	m := chi.NewRouter()

	m.Use(middleware.GetHead)
	m.Use(middleware.Logger)
	m.Use(middleware.Recoverer)

	m.Handle("/_groupcache/*", c.gpool)

	m.Get("/ac/*", c.handleGET())
	m.Get("/cas/*", c.handleGET())

	m.Put("/ac/*", c.handlePUT())
	m.Put("/cas/*", c.handlePUT())

	m.ServeHTTP(rw, r)
}

func (c *cacheServer) handleGET() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[1:]

		var b groupcache.ByteView
		err := c.group.Get(r.Context(), key, groupcache.ByteViewSink(&b))
		if cause := errors.Cause(err); err != nil {
			if awsErr, ok := cause.(awserr.RequestFailure); ok && awsErr.StatusCode() == http.StatusNotFound {
				http.NotFound(rw, r)
				return
			}

			log.Println(errors.Wrap(err, "http get request failed"))
			http.Error(rw, "failed to retrieve key", http.StatusInternalServerError)
		}

		http.ServeContent(rw, r, key, time.Unix(0, 0), b.Reader())
	}
}

func bufferToDisk(tempdir string, source io.Reader) (*os.File, error) {
	f, err := ioutil.TempFile(tempdir, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tempfile for upload buffering")
	}

	if _, err := io.Copy(f, source); err != nil {
		return nil, errors.Wrap(err, "failed to buffer PUT body correctly")
	}

	if _, err := f.Seek(0, 0); err != nil {
		return nil, errors.Wrap(err, "failed to return file offset to the start of the file")
	}

	return f, nil
}

func uploadFile(ctx context.Context, f *os.File, key string, s3m *S3Manager) error {
	if err := s3m.PutReader(ctx, key, f); err != nil {
		return errors.Wrap(err, "upload manager put failed")
	}

	if err := f.Close(); err != nil {
		return errors.Wrap(err, "failed to close body buffer tempfile")
	}

	if err := os.Remove(f.Name()); err != nil {
		return errors.Wrap(err, "failed to cleanup body buffer tempfile")
	}

	return nil
}

func (c *cacheServer) handlePUT() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[1:]

		f, err := bufferToDisk("", r.Body)
		if err != nil {
			e := "failed to buffer to disk"
			log.Println(errors.Wrap(err, e))
			http.Error(rw, e, http.StatusInternalServerError)
			return
		}

		go func() {
			err := uploadFile(context.Background(), f, key, c.s3m)
			if err != nil {
				log.Println(errors.Wrap(err, "failed to upload buffered put file"))
			}
		}()
	}
}
