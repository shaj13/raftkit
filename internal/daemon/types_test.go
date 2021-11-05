// Code generated by MockGen. DO NOT EDIT.
// Source: internal/daemon/types.go

// Package daemon is a generated GoMock package.
package daemon

import (
	context "context"
	io "io"
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	membership "github.com/shaj13/raftkit/internal/membership"
	storage "github.com/shaj13/raftkit/internal/storage"
	transport "github.com/shaj13/raftkit/internal/transport"
	v3 "go.etcd.io/etcd/raft/v3"
)

// MockOperator is a mock of Operator interface.
type MockOperator struct {
	ctrl     *gomock.Controller
	recorder *MockOperatorMockRecorder
}

// MockOperatorMockRecorder is the mock recorder for MockOperator.
type MockOperatorMockRecorder struct {
	mock *MockOperator
}

// NewMockOperator creates a new mock instance.
func NewMockOperator(ctrl *gomock.Controller) *MockOperator {
	mock := &MockOperator{ctrl: ctrl}
	mock.recorder = &MockOperatorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOperator) EXPECT() *MockOperatorMockRecorder {
	return m.recorder
}

// String mocks base method.
func (m *MockOperator) String() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "String")
	ret0, _ := ret[0].(string)
	return ret0
}

// String indicates an expected call of String.
func (mr *MockOperatorMockRecorder) String() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "String", reflect.TypeOf((*MockOperator)(nil).String))
}

// after mocks base method.
func (m *MockOperator) after(ost *operatorsState) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "after", ost)
	ret0, _ := ret[0].(error)
	return ret0
}

// after indicates an expected call of after.
func (mr *MockOperatorMockRecorder) after(ost interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "after", reflect.TypeOf((*MockOperator)(nil).after), ost)
}

// before mocks base method.
func (m *MockOperator) before(ost *operatorsState) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "before", ost)
	ret0, _ := ret[0].(error)
	return ret0
}

// before indicates an expected call of before.
func (mr *MockOperatorMockRecorder) before(ost interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "before", reflect.TypeOf((*MockOperator)(nil).before), ost)
}

// MockConfig is a mock of Config interface.
type MockConfig struct {
	ctrl     *gomock.Controller
	recorder *MockConfigMockRecorder
}

// MockConfigMockRecorder is the mock recorder for MockConfig.
type MockConfigMockRecorder struct {
	mock *MockConfig
}

// NewMockConfig creates a new mock instance.
func NewMockConfig(ctrl *gomock.Controller) *MockConfig {
	mock := &MockConfig{ctrl: ctrl}
	mock.recorder = &MockConfigMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfig) EXPECT() *MockConfigMockRecorder {
	return m.recorder
}

// Context mocks base method.
func (m *MockConfig) Context() context.Context {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Context")
	ret0, _ := ret[0].(context.Context)
	return ret0
}

// Context indicates an expected call of Context.
func (mr *MockConfigMockRecorder) Context() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Context", reflect.TypeOf((*MockConfig)(nil).Context))
}

// Dial mocks base method.
func (m *MockConfig) Dial() transport.Dial {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Dial")
	ret0, _ := ret[0].(transport.Dial)
	return ret0
}

// Dial indicates an expected call of Dial.
func (mr *MockConfigMockRecorder) Dial() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Dial", reflect.TypeOf((*MockConfig)(nil).Dial))
}

// DrainTimeout mocks base method.
func (m *MockConfig) DrainTimeout() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DrainTimeout")
	ret0, _ := ret[0].(time.Duration)
	return ret0
}

// DrainTimeout indicates an expected call of DrainTimeout.
func (mr *MockConfigMockRecorder) DrainTimeout() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DrainTimeout", reflect.TypeOf((*MockConfig)(nil).DrainTimeout))
}

// FSM mocks base method.
func (m *MockConfig) FSM() FSM {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FSM")
	ret0, _ := ret[0].(FSM)
	return ret0
}

// FSM indicates an expected call of FSM.
func (mr *MockConfigMockRecorder) FSM() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FSM", reflect.TypeOf((*MockConfig)(nil).FSM))
}

// Pool mocks base method.
func (m *MockConfig) Pool() membership.Pool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Pool")
	ret0, _ := ret[0].(membership.Pool)
	return ret0
}

// Pool indicates an expected call of Pool.
func (mr *MockConfigMockRecorder) Pool() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Pool", reflect.TypeOf((*MockConfig)(nil).Pool))
}

// RaftConfig mocks base method.
func (m *MockConfig) RaftConfig() *v3.Config {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RaftConfig")
	ret0, _ := ret[0].(*v3.Config)
	return ret0
}

// RaftConfig indicates an expected call of RaftConfig.
func (mr *MockConfigMockRecorder) RaftConfig() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RaftConfig", reflect.TypeOf((*MockConfig)(nil).RaftConfig))
}

// SnapInterval mocks base method.
func (m *MockConfig) SnapInterval() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SnapInterval")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// SnapInterval indicates an expected call of SnapInterval.
func (mr *MockConfigMockRecorder) SnapInterval() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SnapInterval", reflect.TypeOf((*MockConfig)(nil).SnapInterval))
}

// Storage mocks base method.
func (m *MockConfig) Storage() storage.Storage {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Storage")
	ret0, _ := ret[0].(storage.Storage)
	return ret0
}

// Storage indicates an expected call of Storage.
func (mr *MockConfigMockRecorder) Storage() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Storage", reflect.TypeOf((*MockConfig)(nil).Storage))
}

// TickInterval mocks base method.
func (m *MockConfig) TickInterval() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TickInterval")
	ret0, _ := ret[0].(time.Duration)
	return ret0
}

// TickInterval indicates an expected call of TickInterval.
func (mr *MockConfigMockRecorder) TickInterval() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TickInterval", reflect.TypeOf((*MockConfig)(nil).TickInterval))
}

// MockFSM is a mock of FSM interface.
type MockFSM struct {
	ctrl     *gomock.Controller
	recorder *MockFSMMockRecorder
}

// MockFSMMockRecorder is the mock recorder for MockFSM.
type MockFSMMockRecorder struct {
	mock *MockFSM
}

// NewMockFSM creates a new mock instance.
func NewMockFSM(ctrl *gomock.Controller) *MockFSM {
	mock := &MockFSM{ctrl: ctrl}
	mock.recorder = &MockFSMMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFSM) EXPECT() *MockFSMMockRecorder {
	return m.recorder
}

// Apply mocks base method.
func (m *MockFSM) Apply(arg0 []byte) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Apply", arg0)
}

// Apply indicates an expected call of Apply.
func (mr *MockFSMMockRecorder) Apply(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Apply", reflect.TypeOf((*MockFSM)(nil).Apply), arg0)
}

// Restore mocks base method.
func (m *MockFSM) Restore(arg0 io.ReadCloser) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Restore", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Restore indicates an expected call of Restore.
func (mr *MockFSMMockRecorder) Restore(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Restore", reflect.TypeOf((*MockFSM)(nil).Restore), arg0)
}

// Snapshot mocks base method.
func (m *MockFSM) Snapshot() (io.ReadCloser, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Snapshot")
	ret0, _ := ret[0].(io.ReadCloser)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Snapshot indicates an expected call of Snapshot.
func (mr *MockFSMMockRecorder) Snapshot() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Snapshot", reflect.TypeOf((*MockFSM)(nil).Snapshot))
}
