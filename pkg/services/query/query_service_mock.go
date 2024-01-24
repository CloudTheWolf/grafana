// Code generated by mockery v2.14.0. DO NOT EDIT.

package query

import (
	context "context"

	backend "github.com/grafana/grafana-plugin-sdk-go/backend"

	dtos "github.com/grafana/grafana/pkg/api/dtos"

	mock "github.com/stretchr/testify/mock"

	identity "github.com/grafana/grafana/pkg/services/auth/identity"
)

// FakeQueryService is an autogenerated mock type for the Service type
type FakeQueryService struct {
	mock.Mock
}

// QueryData provides a mock function with given fields: ctx, _a1, skipDSCache, reqDTO
func (_m *FakeQueryService) QueryData(ctx context.Context, _a1 identity.Requester, skipDSCache bool, reqDTO dtos.MetricRequest, service string) (*backend.QueryDataResponse, error) {
	ret := _m.Called(ctx, _a1, skipDSCache, reqDTO)

	var r0 *backend.QueryDataResponse
	if rf, ok := ret.Get(0).(func(context.Context, identity.Requester, bool, dtos.MetricRequest) *backend.QueryDataResponse); ok {
		r0 = rf(ctx, _a1, skipDSCache, reqDTO)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*backend.QueryDataResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, identity.Requester, bool, dtos.MetricRequest) error); ok {
		r1 = rf(ctx, _a1, skipDSCache, reqDTO)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Run provides a mock function with given fields: ctx
func (_m *FakeQueryService) Run(ctx context.Context) error {
	ret := _m.Called(ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewFakeQueryService interface {
	mock.TestingT
	Cleanup(func())
}

// NewFakeQueryService creates a new instance of FakeQueryService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewFakeQueryService(t mockConstructorTestingTNewFakeQueryService) *FakeQueryService {
	mock := &FakeQueryService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
