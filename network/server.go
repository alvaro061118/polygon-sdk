package network

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	noise "github.com/libp2p/go-libp2p-noise"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

const DefaultLibp2pPort int = 1478

const (
	MinimumPeerConnections int64 = 1
	MinimumBootNodes       int   = 2 // MinimumBootNodes Count is set to 2 so that, a bootnode can reconnect to the network using other bootnode after restarting.
)

// Priority for dial queue
const (
	PriorityRequestedDial uint64 = 1
	PriorityRandomDial    uint64 = 10
)

type Config struct {
	NoDiscover     bool
	Addr           *net.TCPAddr
	NatAddr        net.IP
	Dns            multiaddr.Multiaddr
	DataDir        string
	MaxPeers       uint64
	Chain          *chain.Chain
	SecretsManager secrets.SecretsManager
}

func DefaultConfig() *Config {
	return &Config{
		NoDiscover: false,
		Addr:       &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: DefaultLibp2pPort},
		MaxPeers:   10,
	}
}

type Server struct {
	logger hclog.Logger
	config *Config

	closeCh chan struct{}

	host  host.Host
	addrs []multiaddr.Multiaddr

	peers     map[peer.ID]*Peer
	peersLock sync.Mutex

	dialQueue *dialQueue

	identity  *identity
	discovery *discovery

	protocols     map[string]Protocol
	protocolsLock sync.Mutex

	// Secrets manager
	secretsManager secrets.SecretsManager

	// pubsub
	ps *pubsub.PubSub

	joinWatchers     map[peer.ID]chan error
	joinWatchersLock sync.Mutex

	emitterPeerEvent event.Emitter
}

type Peer struct {
	srv *Server

	Info peer.AddrInfo
}

// setupLibp2pKey is a helper method for setting up the networking private key
func setupLibp2pKey(secretsManager secrets.SecretsManager) (crypto.PrivKey, error) {
	var key crypto.PrivKey
	if secretsManager.HasSecret(secrets.NetworkKey) {
		// The key is present in the secrets manager, read it
		networkingKey, readErr := ReadLibp2pKey(secretsManager)
		if readErr != nil {
			return nil, fmt.Errorf("unable to read networking private key from Secrets Manager, %v", readErr)
		}

		key = networkingKey
	} else {
		// The key is not present in the secrets manager, generate it
		libp2pKey, libp2pKeyEncoded, keyErr := GenerateAndEncodeLibp2pKey()
		if keyErr != nil {
			return nil, fmt.Errorf("unable to generate networking private key for Secrets Manager, %v", keyErr)
		}

		// Write the networking private key to disk
		if setErr := secretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
			return nil, fmt.Errorf("unable to store networking private key to Secrets Manager, %v", setErr)
		}

		key = libp2pKey
	}

	return key, nil
}

func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	logger = logger.Named("network")

	key, err := setupLibp2pKey(config.SecretsManager)
	if err != nil {
		return nil, err
	}

	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
	}

	addrsFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if config.NatAddr != nil {
			addr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.NatAddr.String(), config.Addr.Port))

			if addr != nil {
				addrs = []multiaddr.Multiaddr{addr}
			}
		} else if config.Dns != nil {
			addrs = []multiaddr.Multiaddr{config.Dns}
		}

		return addrs
	}

	host, err := libp2p.New(
		context.Background(),
		// Use noise as the encryption protocol
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrs(listenAddr),
		libp2p.AddrsFactory(addrsFactory),
		libp2p.Identity(key),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p stack: %v", err)
	}

	emitter, err := host.EventBus().Emitter(new(PeerEvent))
	if err != nil {
		return nil, err
	}

	srv := &Server{
		logger:           logger,
		config:           config,
		host:             host,
		addrs:            host.Addrs(),
		peers:            map[peer.ID]*Peer{},
		dialQueue:        newDialQueue(),
		closeCh:          make(chan struct{}),
		emitterPeerEvent: emitter,
		protocols:        map[string]Protocol{},
		secretsManager:   config.SecretsManager,
	}

	// start identity
	srv.identity = &identity{srv: srv}
	srv.identity.setup()

	// start gossip protocol
	ps, err := pubsub.NewGossipSub(context.Background(), host)
	if err != nil {
		return nil, err
	}
	srv.ps = ps

	return srv, nil
}

func (s *Server) Start() error {
	s.identity.start()

	go s.runDial()
	go s.checkPeerConnections()
	s.logger.Info("LibP2P server running", "addr", AddrInfoToString(s.AddrInfo()))

	if !s.config.NoDiscover {
		if s.config.Chain.Bootnodes != nil && len(s.config.Chain.Bootnodes) < MinimumBootNodes {
			return errors.New("Minimum two bootnodes are required")
		}

		// start discovery
		s.discovery = &discovery{srv: s}

		// try to decode the bootnodes
		bootnodes := []*peer.AddrInfo{}
		for _, raw := range s.config.Chain.Bootnodes {
			node, err := StringToAddrInfo(raw)
			if err != nil {
				return fmt.Errorf("failed to parse bootnode %s: %v", raw, err)
			}
			if node.ID == s.host.ID() {
				s.logger.Info("Omitting bootnode with same ID as host", "id", node.ID)
				continue
			}
			bootnodes = append(bootnodes, node)
		}

		s.discovery.setup(bootnodes)
	}

	go s.runJoinWatcher()

	// watch for disconnected peers
	s.host.Network().Notify(&network.NotifyBundle{
		DisconnectedF: func(net network.Network, conn network.Conn) {
			go func() {
				s.delPeer(conn.RemotePeer())
			}()
		},
	})

	return nil
}

// checkPeerCount will attempt to make new connections if the active peer count is lesser than the specified limit.
func (s *Server) checkPeerConnections() {
	for {
		select {
		case <-time.After(30 * time.Second):
		case <-s.closeCh:
			return
		}
		if s.numPeers() < MinimumPeerConnections {
			if s.config.NoDiscover || len(s.discovery.bootnodes) == 0 {
				//TODO: dial peers from the peerstore
			} else {
				randomNode := s.getRandomBootNode()
				s.addToDialQueue(randomNode, PriorityRandomDial)
			}
		}
	}
}

func (s *Server) runDial() {
	// watch for events of peers included or removed
	notifyCh := make(chan struct{})
	err := s.SubscribeFn(func(evnt *PeerEvent) {
		// only concerned about PeerConnected, PeerFailedToConnect, PeerDisconnected, PeerDialCompleted, and PeerAddedToDialQueue
		switch evnt.Type {
		case PeerConnected, PeerFailedToConnect, PeerDisconnected, PeerDialCompleted, PeerAddedToDialQueue:
		default:
			return
		}

		select {
		case notifyCh <- struct{}{}:
		default:
		}
	})
	if err != nil {
		s.logger.Error("dial manager failed to subscribe", "err", err)
	}

	for {

		// TODO: Right now the dial task are done sequentially because Connect
		// is a blocking request. In the future we should try to make up to
		// maxDials requests concurrently.
		for i := int64(0); i < s.numOpenSlots(); i++ {
			tt := s.dialQueue.pop()
			if tt == nil {
				// dial closed
				return
			}
			s.logger.Debug("dial", "local", s.host.ID(), "addr", tt.addr.String())

			if s.isConnected(tt.addr.ID) {
				// the node is already connected, send an event to wake up
				// any join watchers
				s.emitEvent(tt.addr.ID, PeerAlreadyConnected)
			} else {
				// the connection process is async because it involves connection (here) +
				// the handshake done in the identity service.
				if err := s.host.Connect(context.Background(), *tt.addr); err != nil {
					s.logger.Trace("failed to dial", "addr", tt.addr.String(), "err", err)
				}
			}
		}

		// wait until there is a change in the state of a peer that
		// might involve a new dial slot available
		select {
		case <-notifyCh:
		case <-s.closeCh:
			return
		}
	}
}

func (s *Server) numPeers() int64 {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()
	return int64(len(s.peers))
}
func (s *Server) getRandomBootNode() *peer.AddrInfo {

	return s.discovery.bootnodes[rand.Intn(len(s.discovery.bootnodes))]

}
func (s *Server) Peers() []*Peer {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	peers := make([]*Peer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
	}
	return peers
}

func (s *Server) numOpenSlots() int64 {
	n := int64(s.config.MaxPeers) - (s.numPeers() + s.identity.numPending())
	if n < 0 {
		n = 0
	}
	return n
}

func (s *Server) isConnected(peerID peer.ID) bool {
	return s.host.Network().Connectedness(peerID) == network.Connected
}

func (s *Server) GetProtocols(peerID peer.ID) ([]string, error) {
	return s.host.Peerstore().GetProtocols(peerID)
}

func (s *Server) GetPeerInfo(peerID peer.ID) peer.AddrInfo {
	return s.host.Peerstore().PeerInfo(peerID)
}

func (s *Server) addPeer(id peer.ID) {
	s.logger.Info("Peer connected", "id", id.String())

	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	p := &Peer{
		srv:  s,
		Info: s.host.Peerstore().PeerInfo(id),
	}
	s.peers[id] = p

	s.emitEvent(id, PeerConnected)
}

func (s *Server) delPeer(id peer.ID) {
	s.logger.Info("Peer disconnected", "id", id.String())

	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	delete(s.peers, id)
	s.host.Network().ClosePeer(id)

	s.emitEvent(id, PeerDisconnected)
}

func (s *Server) Disconnect(peer peer.ID, reason string) {
	if s.host.Network().Connectedness(peer) == network.Connected {
		// send some close message
		s.host.Network().ClosePeer(peer)
	}
}

func (s *Server) waitForEvent(timeout time.Duration, handler func(evnt *PeerEvent) bool) bool {
	// TODO: Try to replace joinwatcher with this
	sub, _ := s.Subscribe()

	doneCh := make(chan struct{})
	closed := false
	go func() {
		loop := true
		for loop {
			select {
			case evnt := <-sub.GetCh():
				if handler(evnt) {
					loop = false
				}

			case <-s.closeCh:
				closed = true
				loop = false
			}
		}
		sub.Close()
		doneCh <- struct{}{}
	}()
	if closed {
		return false
	}

	select {
	case <-doneCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

var DefaultJoinTimeout = 10 * time.Second

func (s *Server) JoinAddr(addr string, timeout time.Duration) error {
	addr0, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}
	addr1, err := peer.AddrInfoFromP2pAddr(addr0)
	if err != nil {
		return err
	}
	return s.Join(addr1, timeout)
}

func (s *Server) Join(addr *peer.AddrInfo, timeout time.Duration) error {
	s.logger.Info("Join request", "addr", addr.String())
	s.addToDialQueue(addr, PriorityRequestedDial)

	if timeout == 0 {
		return nil
	}
	err := s.watch(addr.ID, timeout)
	return err
}

func (s *Server) watch(peerID peer.ID, dur time.Duration) error {
	ch := make(chan error)

	s.joinWatchersLock.Lock()
	if s.joinWatchers == nil {
		s.joinWatchers = map[peer.ID]chan error{}
	}
	s.joinWatchers[peerID] = ch
	s.joinWatchersLock.Unlock()

	select {
	case <-time.After(dur):
		s.joinWatchersLock.Lock()
		delete(s.joinWatchers, peerID)
		s.joinWatchersLock.Unlock()

		return fmt.Errorf("timeout %s %s", s.host.ID(), peerID)
	case err := <-ch:
		return err
	}
}

func (s *Server) runJoinWatcher() error {
	return s.SubscribeFn(func(evnt *PeerEvent) {
		switch evnt.Type {
		// only concerned about PeerConnected, PeerFailedToConnect, and PeerAlreadyConnected
		case PeerConnected, PeerFailedToConnect, PeerAlreadyConnected:
		default:
			return
		}

		// try to find a watcher for this peer
		s.joinWatchersLock.Lock()
		errCh, ok := s.joinWatchers[evnt.PeerID]
		if ok {
			errCh <- nil
			delete(s.joinWatchers, evnt.PeerID)
		}
		s.joinWatchersLock.Unlock()
	})
}

func (s *Server) Close() error {
	err := s.host.Close()
	s.dialQueue.Close()
	close(s.closeCh)

	return err
}

func (s *Server) NewProtoStream(proto string, id peer.ID) (interface{}, error) {
	s.protocolsLock.Lock()
	defer s.protocolsLock.Unlock()

	p, ok := s.protocols[proto]
	if !ok {
		return nil, fmt.Errorf("protocol not found: %s", proto)
	}
	stream, err := s.NewStream(proto, id)
	if err != nil {
		return nil, err
	}
	return p.Client(stream), nil
}

func (s *Server) NewStream(proto string, id peer.ID) (network.Stream, error) {
	return s.host.NewStream(context.Background(), id, protocol.ID(proto))
}

type Protocol interface {
	Client(network.Stream) interface{}
	Handler() func(network.Stream)
}

func (s *Server) Register(id string, p Protocol) {
	s.protocolsLock.Lock()
	s.protocols[id] = p
	s.wrapStream(id, p.Handler())
	s.protocolsLock.Unlock()
}

func (s *Server) wrapStream(id string, handle func(network.Stream)) {
	s.host.SetStreamHandler(protocol.ID(id), func(stream network.Stream) {
		peerID := stream.Conn().RemotePeer()
		s.logger.Trace("open stream", "protocol", id, "peer", peerID)

		handle(stream)
	})
}

func (s *Server) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.addrs,
	}
}

func (s *Server) addToDialQueue(addr *peer.AddrInfo, priority uint64) {
	s.dialQueue.add(addr, priority)
	s.emitEvent(addr.ID, PeerAddedToDialQueue)
}

func (s *Server) emitEvent(peerID peer.ID, typ PeerEventType) {
	evnt := PeerEvent{
		PeerID: peerID,
		Type:   typ,
	}

	if err := s.emitterPeerEvent.Emit(evnt); err != nil {
		s.logger.Info("failed to emit event", "peer", evnt.PeerID, "type", evnt.Type, "err", err)
	}
}

type Subscription struct {
	sub event.Subscription
	ch  chan *PeerEvent
}

func (s *Subscription) run() {
	// convert interface{} to *PeerEvent channels
	for {
		evnt := <-s.sub.Out()
		if obj, ok := evnt.(PeerEvent); ok {
			s.ch <- &obj
		}
	}
}

func (s *Subscription) GetCh() chan *PeerEvent {
	return s.ch
}

func (s *Subscription) Get() *PeerEvent {
	obj := <-s.ch
	return obj
}

func (s *Subscription) Close() {
	s.sub.Close()
}

// Subscribe starts a PeerEvent subscription
func (s *Server) Subscribe() (*Subscription, error) {
	raw, err := s.host.EventBus().Subscribe(new(PeerEvent))
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		sub: raw,
		ch:  make(chan *PeerEvent),
	}
	go sub.run()
	return sub, nil
}

// SubscribeFn is a helper method to run subscription of PeerEvents
func (s *Server) SubscribeFn(handler func(evnt *PeerEvent)) error {
	sub, err := s.Subscribe()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case evnt := <-sub.GetCh():
				handler(evnt)

			case <-s.closeCh:
				sub.Close()
				return
			}
		}
	}()
	return nil
}

// SubscribeCh returns an event of of subscription events
func (s *Server) SubscribeCh() (<-chan *PeerEvent, error) {
	ch := make(chan *PeerEvent)

	var closed bool
	var mutex sync.Mutex

	isClosed := func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return closed
	}

	err := s.SubscribeFn(func(evnt *PeerEvent) {
		if !isClosed() {
			ch <- evnt
		}
	})
	if err != nil {
		close(ch)
		return nil, err
	}

	go func() {
		<-s.closeCh
		mutex.Lock()
		closed = true
		mutex.Unlock()
		close(ch)
	}()

	return ch, nil
}

func StringToAddrInfo(addr string) (*peer.AddrInfo, error) {
	addr0, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return nil, err
	}
	addr1, err := peer.AddrInfoFromP2pAddr(addr0)
	if err != nil {
		return nil, err
	}
	return addr1, nil
}

var (
	// Regex used for matching loopback addresses (IPv4 and IPv6)
	// This regex will match:
	// /ip4/localhost/tcp/<port>
	// /ip4/127.0.0.1/tcp/<port>
	// /ip4/<any other loopback>/tcp/<port>
	// /ip6/<any loopback>/tcp/<port>
	loopbackRegex = regexp.MustCompile(
		`^\/ip4\/127(?:\.[0-9]+){0,2}\.[0-9]+\/tcp\/\d+$|^\/ip4\/localhost\/tcp\/\d+$|^\/ip6\/(?:0*\:)*?:?0*1\/tcp\/\d+$`,
	)
)

// AddrInfoToString converts an AddrInfo into a string representation that can be dialed from another node
func AddrInfoToString(addr *peer.AddrInfo) string {
	// Safety check
	if len(addr.Addrs) == 0 {
		panic("No dial addresses found")
	}

	dialAddress := addr.Addrs[0].String()

	// Try to see if a non loopback address is present in the list
	if len(addr.Addrs) > 1 && loopbackRegex.MatchString(dialAddress) {
		// Find an address that's not a loopback address
		for _, address := range addr.Addrs {
			if !loopbackRegex.MatchString(address.String()) {
				// Not a loopback address, dial address found
				dialAddress = address.String()
				break
			}
		}
	}

	// Format output and return
	return dialAddress + "/p2p/" + addr.ID.String()
}

type PeerEventType uint

const (
	PeerConnected        PeerEventType = iota // Emitted when a peer connected
	PeerFailedToConnect                       // Emitted when a peer failed to connect
	PeerDisconnected                          // Emitted when a peer disconnected from node
	PeerAlreadyConnected                      // Emitted when a peer already connected on dial
	PeerDialCompleted                         // Emitted when a peer completed dial
	PeerAddedToDialQueue                      // Emitted when a peer is added to dial queue
)

var peerEventToName = map[PeerEventType]string{
	PeerConnected:        "PeerConnected",
	PeerFailedToConnect:  "PeerFailedToConnect",
	PeerDisconnected:     "PeerDisconnected",
	PeerAlreadyConnected: "PeerAlreadyConnected",
	PeerDialCompleted:    "PeerDialCompleted",
	PeerAddedToDialQueue: "PeerAddedToDialQueue",
}

func (s PeerEventType) String() string {
	name, ok := peerEventToName[s]
	if !ok {
		return "unknown"
	}
	return name
}

type PeerEvent struct {
	// PeerID is the id of the peer that triggered
	// the event
	PeerID peer.ID

	// Type is the type of the event
	Type PeerEventType
}
