package main

import (
	"log"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/golang/groupcache"
	"github.com/pkg/errors"
)

// Updater is responsible for setting the peers for a given pool, it may block and do this indefinitely or simply run once.
type Updater func(p *groupcache.HTTPPool) error

func selfInPeers(self string, peers []string) bool {
	for _, peer := range peers {
		if peer == self {
			return true
		}
	}
	return false
}

// StaticPeers validates and then sets the peers for a groupcaache.HTTPPool to be the provided peers
func StaticPeers(self string, peers []string) Updater {
	return func(pool *groupcache.HTTPPool) error {
		for i, peer := range peers {
			_, err := url.Parse(peer)
			if err != nil {
				return errors.Wrapf(err, "failed to parse peer URL %q:%q", i, peer)
			}
		}

		if !selfInPeers(self, peers) {
			return errors.Errorf("self not in peers: %q not in %q", self, peers)
		}

		pool.Set(peers...)

		return nil
	}
}

func srvLookup(srvName string, srvPortName string) ([]string, error) {
	cname, targets, err := net.LookupSRV(srvPortName, "tcp", srvName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve SRV record")
	}
	log.Printf("SRV Name: %q", cname)

	// Build peer list from SRV targets
	peers := make([]string, len(targets))
	for i, addr := range targets {
		ip, err := net.LookupHost(addr.Target)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve SRV record")
		}

		if len(ip) != 1 {
			return nil, errors.Errorf("Got multiple hosts for ip: %q in %q", addr.Target, ip)
		}

		peers[i] = (&url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(ip[0], strconv.Itoa(int(addr.Port))),
		}).String()
		log.Printf("SRV peer: %q", peers[i])
	}

	return peers, nil
}

// SRVDiscoveredPeers periodically sends SRV requests to the provided DNS name to discover (& set) the pool's peers
func SRVDiscoveredPeers(self string, srvPeerDNSName string, srvPortName string, updateInterval time.Duration) Updater {
	update := func(pool *groupcache.HTTPPool) error {
		peers, err := srvLookup(srvPeerDNSName, srvPortName)
		if err != nil {
			pool.Set(self)
			return nil
			// return errors.Wrap(err, "srv lookup failed")
		}

		if !selfInPeers(self, peers) {
			// return errors.Errorf("self not in peers: %q not in %q", self, peers)
		}

		pool.Set(peers...)

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
