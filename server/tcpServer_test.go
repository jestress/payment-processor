package server

import (
	"context"
	"testing"

	"github.com/jestress/payment-processor/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_TcpServer_NewTcpServer_ReturnsTcpServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	_, cn := context.WithCancel(t.Context())

	mock_requestHandler := mock.NewMockRequestHandler(ctrl)

	tcpServer, err := NewTcpServer(mock_requestHandler, cn)

	assert.NotNil(t, tcpServer)
	assert.NoError(t, err)

	tcpServer.Stop()
}

func Test_TcpServer_GetExitChannel_ReturnsExitChannel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	_, cn := context.WithCancel(t.Context())

	mock_requestHandler := mock.NewMockRequestHandler(ctrl)

	tcpServer, _ := NewTcpServer(mock_requestHandler, cn)

	tcpServer.Stop()
}
