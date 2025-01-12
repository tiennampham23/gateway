package handler

import (
	"runtime"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

type testHandler struct {
	*BxConn
}

// testHandler immediately closes the connection when a message is received
func (th *testHandler) ProcessMessage(msg bxmessage.MessageBytes) {
	_ = th.BxConn.Close("message handler test")
}

func (th *testHandler) setConn(b *BxConn) {
	th.BxConn = b
}

func TestBxConn_BDNIsDefaultForOldProtocol(t *testing.T) {
	th := testHandler{}
	_, bx := bxConn(&th)
	th.setConn(bx)

	helloMessage := bxmessage.Hello{}
	b, _ := helloMessage.Pack(bxmessage.FlashbotsGatewayProtocol - 1)
	msg := bxmessage.NewMessageBytes(b, time.Now())
	bx.ProcessMessage(msg)

	assert.True(t, bx.capabilities&types.CapabilityBDN != 0)
}

// semi integration test: in general, sleep should be avoided, but these closing tests cases are checking that we are closing goroutines correctly
func TestBxConn_ClosingFromHandler(t *testing.T) {
	startCount := runtime.NumGoroutine()

	th := testHandler{}
	tls, bx := bxConn(&th)
	th.setConn(bx)

	err := bx.Start()
	assert.Nil(t, err)

	// wait for hello message to be sent on connection so all goroutines are started
	_, err = tls.MockAdvanceSent()
	assert.Nil(t, err)

	// expect 3 additional goroutines: read loop, send loop and read from receive channel
	assert.Equal(t, startCount+3, runtime.NumGoroutine())

	// queue message, which should trigger a close
	helloMessage := bxmessage.Hello{}
	b, err := helloMessage.Pack(bxmessage.CurrentProtocol)
	tls.MockQueue(b)

	// allow small delta for goroutines to finish
	time.Sleep(1 * time.Millisecond)

	endCount := runtime.NumGoroutine()
	assert.Equal(t, startCount, endCount)
}

func bxConn(handler connections.ConnHandler) (bxmock.MockTLS, *BxConn) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := bxmock.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := utils.TestCerts()
	b := NewBxConn(bxmock.MockBxListener{},
		func() (connections.Socket, error) {
			return tls, nil
		},
		handler, &certs, ip, port, "", utils.RelayTransaction, true, false, true, false, connections.LocalInitiatedPort, utils.RealClock{},
		false)
	return tls, b
}
