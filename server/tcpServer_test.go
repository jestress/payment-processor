package server

import (
	"testing"

	"github.com/form3tech-oss/interview-simulator/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_TcpServer_NewTcpServer_ReturnsTcpServer(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock_requestHandler := mock.NewMockRequestHandler(ctrl)

	// Act
	tcpServer, err := NewTcpServer(mock_requestHandler)

	// Assert
	assert.NotNil(t, tcpServer)
	assert.NoError(t, err)

	tcpServer.Stop()
}

func Test_TcpServer_GetExitChannel_ReturnsExitChannel(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock_requestHandler := mock.NewMockRequestHandler(ctrl)

	tcpServer, _ := NewTcpServer(mock_requestHandler)

	// Act
	result := tcpServer.GetExitChannel()

	// Assert
	assert.NotNil(t, result)
	tcpServer.Stop()
}
