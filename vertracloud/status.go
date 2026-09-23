package vertracloud

import (
	"context"
	"net/http"

	"github.com/vertracloud/sdk-api-go/rest"
)

type StatusInfo struct {
	Status  StatusType `json:"status"`
	Message string     `json:"message"`
}

// StatusType is the status returned by GET /v1/status.
type StatusType string

const (
	StatusTypeHealthy  StatusType = "healthy"
	StatusTypeDegraded StatusType = "degraded"
	StatusTypeUnknown  StatusType = "unknown"
)

type StatusService interface {
	Get(ctx context.Context, opts ...rest.RequestOpt) (StatusInfo, error)
}
type statusServiceImpl struct{ rest rest.Client }

func newStatusService(rc rest.Client) StatusService { return &statusServiceImpl{rest: rc} }
func (s *statusServiceImpl) Get(ctx context.Context, opts ...rest.RequestOpt) (StatusInfo, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/status", nil, nil, "", opts...)
	if err != nil {
		return StatusInfo{}, err
	}
	return rest.DecodeJSON[StatusInfo](data)
}
