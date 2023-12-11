// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/ava-labs/avalanchego/vms/platformvm/txs/mempool (interfaces: Mempool)

// Package mempool is a generated GoMock package.
package mempool

import (
	reflect "reflect"
	time "time"

	ids "github.com/ava-labs/avalanchego/ids"
	txs "github.com/ava-labs/avalanchego/vms/platformvm/txs"
	gomock "go.uber.org/mock/gomock"
)

// MockMempool is a mock of Mempool interface.
type MockMempool struct {
	ctrl     *gomock.Controller
	recorder *MockMempoolMockRecorder
}

// MockMempoolMockRecorder is the mock recorder for MockMempool.
type MockMempoolMockRecorder struct {
	mock *MockMempool
}

// NewMockMempool creates a new mock instance.
func NewMockMempool(ctrl *gomock.Controller) *MockMempool {
	mock := &MockMempool{ctrl: ctrl}
	mock.recorder = &MockMempoolMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMempool) EXPECT() *MockMempoolMockRecorder {
	return m.recorder
}

// Add mocks base method.
func (m *MockMempool) Add(arg0 *txs.Tx) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Add", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Add indicates an expected call of Add.
func (mr *MockMempoolMockRecorder) Add(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Add", reflect.TypeOf((*MockMempool)(nil).Add), arg0)
}

// DropExpiredStakerTxs mocks base method.
func (m *MockMempool) DropExpiredStakerTxs(arg0 time.Time) []ids.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DropExpiredStakerTxs", arg0)
	ret0, _ := ret[0].([]ids.ID)
	return ret0
}

// DropExpiredStakerTxs indicates an expected call of DropExpiredStakerTxs.
func (mr *MockMempoolMockRecorder) DropExpiredStakerTxs(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DropExpiredStakerTxs", reflect.TypeOf((*MockMempool)(nil).DropExpiredStakerTxs), arg0)
}

// Get mocks base method.
func (m *MockMempool) Get(arg0 ids.ID) *txs.Tx {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", arg0)
	ret0, _ := ret[0].(*txs.Tx)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockMempoolMockRecorder) Get(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockMempool)(nil).Get), arg0)
}

// GetDropReason mocks base method.
func (m *MockMempool) GetDropReason(arg0 ids.ID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDropReason", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetDropReason indicates an expected call of GetDropReason.
func (mr *MockMempoolMockRecorder) GetDropReason(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDropReason", reflect.TypeOf((*MockMempool)(nil).GetDropReason), arg0)
}

// Has mocks base method.
func (m *MockMempool) Has(arg0 ids.ID) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Has", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Has indicates an expected call of Has.
func (mr *MockMempoolMockRecorder) Has(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Has", reflect.TypeOf((*MockMempool)(nil).Has), arg0)
}

// HasTxs mocks base method.
func (m *MockMempool) HasTxs() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HasTxs")
	ret0, _ := ret[0].(bool)
	return ret0
}

// HasTxs indicates an expected call of HasTxs.
func (mr *MockMempoolMockRecorder) HasTxs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HasTxs", reflect.TypeOf((*MockMempool)(nil).HasTxs))
}

// MarkDropped mocks base method.
func (m *MockMempool) MarkDropped(arg0 ids.ID, arg1 error) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "MarkDropped", arg0, arg1)
}

// MarkDropped indicates an expected call of MarkDropped.
func (mr *MockMempoolMockRecorder) MarkDropped(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MarkDropped", reflect.TypeOf((*MockMempool)(nil).MarkDropped), arg0, arg1)
}

// PeekTxs mocks base method.
func (m *MockMempool) PeekTxs(arg0 int) []*txs.Tx {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PeekTxs", arg0)
	ret0, _ := ret[0].([]*txs.Tx)
	return ret0
}

// PeekTxs indicates an expected call of PeekTxs.
func (mr *MockMempoolMockRecorder) PeekTxs(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PeekTxs", reflect.TypeOf((*MockMempool)(nil).PeekTxs), arg0)
}

// Remove mocks base method.
func (m *MockMempool) Remove(arg0 []*txs.Tx) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Remove", arg0)
}

// Remove indicates an expected call of Remove.
func (mr *MockMempoolMockRecorder) Remove(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Remove", reflect.TypeOf((*MockMempool)(nil).Remove), arg0)
}

// RequestBuildBlock mocks base method.
func (m *MockMempool) RequestBuildBlock(arg0 bool) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RequestBuildBlock", arg0)
}

// RequestBuildBlock indicates an expected call of RequestBuildBlock.
func (mr *MockMempoolMockRecorder) RequestBuildBlock(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RequestBuildBlock", reflect.TypeOf((*MockMempool)(nil).RequestBuildBlock), arg0)
}
