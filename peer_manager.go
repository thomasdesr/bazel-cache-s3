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

type updater func(p *groupcache.HTTPPool) error

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
func StaticPeers(peers []string) func(pool *groupcache.HTTPPool) error {
	return func(pool *groupcache.HTTPPool) error {
		for i, peer := range peers {
			_, err := url.Parse(peer)
			if err != nil {
				return errors.Wrapf(err, "failed to parse peer URL %q:%q", i, peer)
			}
		}

		setPeers(pool, peers)

		return nil
	}
}

func srvLookup(srvName string) ([]string, error) {
	cname, targets, err := net.LookupSRV("bazelcache", "tcp", srvName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve SRV record")
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

	return peers, nil
}

// SRVDiscoveredPeers periodically (defaults to 15s) sends SRV requests to the provided DNS name to discover (& set) the pool's peers
func SRVDiscoveredPeers(self string, srvPeerDNSName string, updateInterval time.Duration) func(pool *groupcache.HTTPPool) error {
	if updateInterval == time.Duration(0) {
		updateInterval = time.Second * 15
	}

	update := func(pool *groupcache.HTTPPool) error {
		peers, err := srvLookup(srvPeerDNSName)
		if err != nil {
			return errors.Wrap(err, "srv lookup failed")
		}

		if !selfInPeers(self, peers) {
			return errors.Errorf("self(%q) is not in peers (%q)", self, peers)
		}

		setPeers(pool, peers)

		return nil
	}

	return func(pool *groupcache.HTTPPool) error {
		if err := update(pool); err != nil {
			return errors.Wrap(err, "initial SRV discovery failed")
		}

		for range time.Tick(updateInterval) {
			if err := update(pool); err != nil {
				log.Println(errors.Wrap(err, "update failed"))
			}
		}

		panic("Time.Tick stopped ticking?!?")
	}
}
