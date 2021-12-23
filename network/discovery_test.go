package network

import (
	"fmt"
	"github.com/hashicorp/go-hclog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func discoveryConfig(c *Config) {
	// we limit maxPeers=1 to limit the number of connections
	// since we only want to test discovery
	c.MaxPeers = 2
}

func TestDiscovery_ConnectedPopulatesRoutingTable(t *testing.T) {
	// when two nodes connect, they populate their kademlia routing tables
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	MultiJoin(t, srv0, srv1)
	time.Sleep(1 * time.Second)

	assert.Equal(t, srv0.discovery.routingTable.Size(), 1)
	assert.Equal(t, srv1.discovery.routingTable.Size(), 1)
}

func TestDiscovery_ProtocolFindPeers(t *testing.T) {
	numServers := 2
	srvs := make([]*Server, numServers)
	for i := 0; i < numServers; i++ {
		srv := CreateServer(t, nil)
		srv.logger = hclog.New(&hclog.LoggerOptions{
			Name:  fmt.Sprintf("protocol-find-peers-%d", i),
			Level: hclog.LevelFromString("DEBUG"),
		})

		srvs[i] = srv
	}

	MultiJoin(t, srvs[0], srvs[1])
	time.Sleep(time.Second * 2)

	// find peers should not include our identity
	resp, err := srvs[0].discovery.findPeersCall(srvs[1].AddrInfo().ID)
	assert.NoError(t, err)
	assert.Empty(t, resp)
}

func TestDiscovery_PeerAdded(t *testing.T) {
	numServers := 3
	srvs := make([]*Server, numServers)
	for i := 0; i < numServers; i++ {
		srv := CreateServer(t, discoveryConfig)
		srv.logger = hclog.New(&hclog.LoggerOptions{
			Name:  fmt.Sprintf("discovery-peer-added-%d", i),
			Level: hclog.LevelFromString("DEBUG"),
		})

		srvs[i] = srv
	}

	// server0 should connect to server2 by discovery
	connectedCh := asyncWaitForEvent(srvs[0], 15*time.Second, connectedPeerHandler(srvs[2].AddrInfo().ID))

	// serial join, srv0 -> srv1 -> srv2
	MultiJoin(t,
		srvs[0], srvs[1],
		srvs[1], srvs[2],
	)
	time.Sleep(time.Second * 2)

	// wait until server0 connects to server2
	assert.True(t, <-connectedCh)
	assert.Len(t, srvs[0].host.Peerstore().Peers(), 3)
	assert.Len(t, srvs[1].host.Peerstore().Peers(), 3)
	assert.Len(t, srvs[2].host.Peerstore().Peers(), 3)

	// TODO: We should put MaxPeers to 0 or 1 so that we do not
	// mix data and we only test how the peers are being populated
	// In theory, even if they are connected only to one peer, all of them
	// should end up with the same idea of the network.
}

func TestDiscovery_FullNetwork(t *testing.T) {
	t.Skip()

	// create a network of serially connected nodes
	// eventually, they have to find each other

	nodes := 20
	servers := []*Server{}
	for i := 0; i < nodes; i++ {
		srv := CreateServer(t, discoveryConfig)
		servers = append(servers, srv)
	}

	// link nodes in serial
	MultiJoinSerial(t, servers)

	// force the discover of other nodes several times
	for i := 0; i < 50; i++ {
		for _, srv := range servers {
			srv.discovery.handleDiscovery()
		}
	}

	for _, srv := range servers {
		fmt.Println("-- peerstore --")
		fmt.Println(srv.host.Peerstore().Peers().Len())
	}
}
