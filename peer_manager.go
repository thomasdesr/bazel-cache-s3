package main

import (
	"log"
	"net"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

func selfInPeers(self string, peers []string) bool {
	for _, peer := range peers {
		if peer == self {
			return true
		}
	}
	return false
}

func setPeers(pool *groupcache.HTTPPool, peers []string) {
	sort.Strings(peers)
	pool.Set(peers...)
}

// StaticPeers validates and then sets the peers for a groupcaache.HTTPPool to be the provided peers
func StaticPeers(pool *groupcache.HTTPPool, peers []string) error {
	for i, peer := range peers {
		_, err := url.Parse(peer)
		if err != nil {
			return errors.Wrapf(err, "failed to parse peer URL %q:%q", i, peer)
		}
	}

	setPeers(pool, peers)

	return nil
}

// SRVDiscoveredPeers periodically (15s) sends SRV requests to the provided DNS name to discover (& set) the pool's peers
func SRVDiscoveredPeers(pool *groupcache.HTTPPool, self string, srvPeerDNSName string) error {
	update := func() error {
		cname, targets, err := net.LookupSRV("bazelcache", "tcp", srvPeerDNSName)
		if err != nil {
			return errors.Wrap(err, "failed to resolve SRV record")
		}
		log.Printf("SRV CNAME: %q", cname)

		// Build peer list from SRV targets
		peers := make([]string, len(targets))
		for i, addr := range targets {
			peers[i] = (&url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(addr.Target, strconv.Itoa(int(addr.Port))),
			}).String()
			log.Printf("SRV peer: %q", peers[i])
		}

		if !selfInPeers(self, peers) {
			return errors.Errorf("self(%q) is not in peers (%q)", self, peers)
		}

		setPeers(pool, peers)

		return nil
	}

	if err := update(); err != nil {
		return errors.Wrap(err, "initial SRV discovery failed")
	}

	for range time.Tick(time.Second * 15) {
		if err := update(); err != nil {
			log.Println(errors.Wrap(err, "update failed"))
		}
	}

	panic("Time.Tick stopped ticking?!?")
}
