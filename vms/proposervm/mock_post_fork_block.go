// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/ava-labs/avalanchego/vms/proposervm (interfaces: PostForkBlock)

// Package proposervm is a generated GoMock package.
package proposervm

import (
	context "context"
	reflect "reflect"
	time "time"

	ids "github.com/ava-labs/avalanchego/ids"
	choices "github.com/ava-labs/avalanchego/snow/choices"
	snowman "github.com/ava-labs/avalanchego/snow/consensus/snowman"
	block "github.com/ava-labs/avalanchego/vms/proposervm/block"
	gomock "go.uber.org/mock/gomock"
)

// MockPostForkBlock is a mock of PostForkBlock interface.
type MockPostForkBlock struct {
	ctrl     *gomock.Controller
	recorder *MockPostForkBlockMockRecorder
}

// MockPostForkBlockMockRecorder is the mock recorder for MockPostForkBlock.
type MockPostForkBlockMockRecorder struct {
	mock *MockPostForkBlock
}

// NewMockPostForkBlock creates a new mock instance.
func NewMockPostForkBlock(ctrl *gomock.Controller) *MockPostForkBlock {
	mock := &MockPostForkBlock{ctrl: ctrl}
	mock.recorder = &MockPostForkBlockMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPostForkBlock) EXPECT() *MockPostForkBlockMockRecorder {
	return m.recorder
}

// Accept mocks base method.
func (m *MockPostForkBlock) Accept(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Accept", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Accept indicates an expected call of Accept.
func (mr *MockPostForkBlockMockRecorder) Accept(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Accept", reflect.TypeOf((*MockPostForkBlock)(nil).Accept), arg0)
}

// Bytes mocks base method.
func (m *MockPostForkBlock) Bytes() []byte {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Bytes")
	ret0, _ := ret[0].([]byte)
	return ret0
}

// Bytes indicates an expected call of Bytes.
func (mr *MockPostForkBlockMockRecorder) Bytes() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Bytes", reflect.TypeOf((*MockPostForkBlock)(nil).Bytes))
}

// Height mocks base method.
func (m *MockPostForkBlock) Height() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Height")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// Height indicates an expected call of Height.
func (mr *MockPostForkBlockMockRecorder) Height() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Height", reflect.TypeOf((*MockPostForkBlock)(nil).Height))
}

// ID mocks base method.
func (m *MockPostForkBlock) ID() ids.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ID")
	ret0, _ := ret[0].(ids.ID)
	return ret0
}

// ID indicates an expected call of ID.
func (mr *MockPostForkBlockMockRecorder) ID() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ID", reflect.TypeOf((*MockPostForkBlock)(nil).ID))
}

// Parent mocks base method.
func (m *MockPostForkBlock) Parent() ids.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Parent")
	ret0, _ := ret[0].(ids.ID)
	return ret0
}

// Parent indicates an expected call of Parent.
func (mr *MockPostForkBlockMockRecorder) Parent() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Parent", reflect.TypeOf((*MockPostForkBlock)(nil).Parent))
}

// Reject mocks base method.
func (m *MockPostForkBlock) Reject(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Reject", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Reject indicates an expected call of Reject.
func (mr *MockPostForkBlockMockRecorder) Reject(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Reject", reflect.TypeOf((*MockPostForkBlock)(nil).Reject), arg0)
}

// Status mocks base method.
func (m *MockPostForkBlock) Status() choices.Status {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Status")
	ret0, _ := ret[0].(choices.Status)
	return ret0
}

// Status indicates an expected call of Status.
func (mr *MockPostForkBlockMockRecorder) Status() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Status", reflect.TypeOf((*MockPostForkBlock)(nil).Status))
}

// Timestamp mocks base method.
func (m *MockPostForkBlock) Timestamp() time.Time {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Timestamp")
	ret0, _ := ret[0].(time.Time)
	return ret0
}

// Timestamp indicates an expected call of Timestamp.
func (mr *MockPostForkBlockMockRecorder) Timestamp() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Timestamp", reflect.TypeOf((*MockPostForkBlock)(nil).Timestamp))
}

// Verify mocks base method.
func (m *MockPostForkBlock) Verify(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Verify", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Verify indicates an expected call of Verify.
func (mr *MockPostForkBlockMockRecorder) Verify(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Verify", reflect.TypeOf((*MockPostForkBlock)(nil).Verify), arg0)
}

// acceptInnerBlk mocks base method.
func (m *MockPostForkBlock) acceptInnerBlk(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "acceptInnerBlk", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// acceptInnerBlk indicates an expected call of acceptInnerBlk.
func (mr *MockPostForkBlockMockRecorder) acceptInnerBlk(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "acceptInnerBlk", reflect.TypeOf((*MockPostForkBlock)(nil).acceptInnerBlk), arg0)
}

// acceptOuterBlk mocks base method.
func (m *MockPostForkBlock) acceptOuterBlk() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "acceptOuterBlk")
	ret0, _ := ret[0].(error)
	return ret0
}

// acceptOuterBlk indicates an expected call of acceptOuterBlk.
func (mr *MockPostForkBlockMockRecorder) acceptOuterBlk() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "acceptOuterBlk", reflect.TypeOf((*MockPostForkBlock)(nil).acceptOuterBlk))
}

// buildChild mocks base method.
func (m *MockPostForkBlock) buildChild(arg0 context.Context) (Block, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "buildChild", arg0)
	ret0, _ := ret[0].(Block)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// buildChild indicates an expected call of buildChild.
func (mr *MockPostForkBlockMockRecorder) buildChild(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "buildChild", reflect.TypeOf((*MockPostForkBlock)(nil).buildChild), arg0)
}

// getInnerBlk mocks base method.
func (m *MockPostForkBlock) getInnerBlk() snowman.Block {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getInnerBlk")
	ret0, _ := ret[0].(snowman.Block)
	return ret0
}

// getInnerBlk indicates an expected call of getInnerBlk.
func (mr *MockPostForkBlockMockRecorder) getInnerBlk() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getInnerBlk", reflect.TypeOf((*MockPostForkBlock)(nil).getInnerBlk))
}

// getStatelessBlk mocks base method.
func (m *MockPostForkBlock) getStatelessBlk() block.Block {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getStatelessBlk")
	ret0, _ := ret[0].(block.Block)
	return ret0
}

// getStatelessBlk indicates an expected call of getStatelessBlk.
func (mr *MockPostForkBlockMockRecorder) getStatelessBlk() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getStatelessBlk", reflect.TypeOf((*MockPostForkBlock)(nil).getStatelessBlk))
}

// pChainHeight mocks base method.
func (m *MockPostForkBlock) pChainHeight(arg0 context.Context) (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "pChainHeight", arg0)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// pChainHeight indicates an expected call of pChainHeight.
func (mr *MockPostForkBlockMockRecorder) pChainHeight(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "pChainHeight", reflect.TypeOf((*MockPostForkBlock)(nil).pChainHeight), arg0)
}

// setInnerBlk mocks base method.
func (m *MockPostForkBlock) setInnerBlk(arg0 snowman.Block) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "setInnerBlk", arg0)
}

// setInnerBlk indicates an expected call of setInnerBlk.
func (mr *MockPostForkBlockMockRecorder) setInnerBlk(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "setInnerBlk", reflect.TypeOf((*MockPostForkBlock)(nil).setInnerBlk), arg0)
}

// setStatus mocks base method.
func (m *MockPostForkBlock) setStatus(arg0 choices.Status) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "setStatus", arg0)
}

// setStatus indicates an expected call of setStatus.
func (mr *MockPostForkBlockMockRecorder) setStatus(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "setStatus", reflect.TypeOf((*MockPostForkBlock)(nil).setStatus), arg0)
}

// verifyPostForkChild mocks base method.
func (m *MockPostForkBlock) verifyPostForkChild(arg0 context.Context, arg1 *postForkBlock) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "verifyPostForkChild", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// verifyPostForkChild indicates an expected call of verifyPostForkChild.
func (mr *MockPostForkBlockMockRecorder) verifyPostForkChild(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "verifyPostForkChild", reflect.TypeOf((*MockPostForkBlock)(nil).verifyPostForkChild), arg0, arg1)
}

// verifyPostForkOption mocks base method.
func (m *MockPostForkBlock) verifyPostForkOption(arg0 context.Context, arg1 *postForkOption) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "verifyPostForkOption", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// verifyPostForkOption indicates an expected call of verifyPostForkOption.
func (mr *MockPostForkBlockMockRecorder) verifyPostForkOption(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "verifyPostForkOption", reflect.TypeOf((*MockPostForkBlock)(nil).verifyPostForkOption), arg0, arg1)
}

// verifyPreForkChild mocks base method.
func (m *MockPostForkBlock) verifyPreForkChild(arg0 context.Context, arg1 *preForkBlock) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "verifyPreForkChild", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// verifyPreForkChild indicates an expected call of verifyPreForkChild.
func (mr *MockPostForkBlockMockRecorder) verifyPreForkChild(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "verifyPreForkChild", reflect.TypeOf((*MockPostForkBlock)(nil).verifyPreForkChild), arg0, arg1)
}
