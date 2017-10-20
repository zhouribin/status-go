// Code generated by MockGen. DO NOT EDIT.
// Source: geth/node/logger.go

// Package node is a generated GoMock package.
package node

import (
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// Mocklogger is a mock of logger interface
type Mocklogger struct {
	ctrl     *gomock.Controller
	recorder *MockloggerMockRecorder
}

// MockloggerMockRecorder is the mock recorder for Mocklogger
type MockloggerMockRecorder struct {
	mock *Mocklogger
}

// NewMocklogger creates a new mock instance
func NewMocklogger(ctrl *gomock.Controller) *Mocklogger {
	mock := &Mocklogger{ctrl: ctrl}
	mock.recorder = &MockloggerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *Mocklogger) EXPECT() *MockloggerMockRecorder {
	return m.recorder
}

// Init mocks base method
func (m *Mocklogger) Init(file, level string) {
	m.ctrl.Call(m, "Init", file, level)
}

// Init indicates an expected call of Init
func (mr *MockloggerMockRecorder) Init(file, level interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Init", reflect.TypeOf((*Mocklogger)(nil).Init), file, level)
}