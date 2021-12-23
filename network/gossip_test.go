package network

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"testing"
	"time"

	testproto "github.com/0xPolygon/polygon-sdk/network/proto/test"
	"github.com/stretchr/testify/assert"
)

func NumSubscribers(srv *Server, topic string) int {
	return len(srv.ps.ListPeers(topic))
}

func WaitForSubscribers(ctx context.Context, srv *Server, topic string, expectedNumPeers int) error {
	for {
		if n := NumSubscribers(srv, topic); n >= expectedNumPeers {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("canceled")
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}

}

func TestGossip(t *testing.T) {
	numServers := 2
	srvs := make([]*Server, numServers)
	for i := 0; i < numServers; i++ {
		srv := CreateServer(t, nil)
		srv.logger = hclog.New(&hclog.LoggerOptions{
			Name:  fmt.Sprintf("gossip-%d", i),
			Level: hclog.LevelFromString("DEBUG"),
		})

		srvs[i] = srv
	}

	MultiJoin(t, srvs[0], srvs[1])

	topicName := "topic/0.1"

	topic0, err := srvs[0].NewTopic(topicName, &testproto.AReq{})
	assert.NoError(t, err)

	topic1, err := srvs[1].NewTopic(topicName, &testproto.AReq{})
	assert.NoError(t, err)

	// subscribe in topic1
	msgCh := make(chan *testproto.AReq)
	if subscribeErr := topic1.Subscribe(func(obj interface{}) {
		msgCh <- obj.(*testproto.AReq)
	}); subscribeErr != nil {
		t.Fatalf("Unable to subscribe to topic, %v", subscribeErr)
	}

	// wait until build mesh
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if waitErr := WaitForSubscribers(ctx, srvs[0], topicName, 1); waitErr != nil {
		t.Fatalf("Unable to wait for subscribers, %v", waitErr)
	}

	// publish in topic0
	assert.NoError(t, topic0.Publish(&testproto.AReq{Msg: "a"}))

	select {
	case msg := <-msgCh:
		assert.Equal(t, msg.Msg, "a")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
