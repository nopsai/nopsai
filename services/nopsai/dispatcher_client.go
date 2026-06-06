package nopsai

import (
	"context"

	"nopsai/pkg/proto"

	"google.golang.org/protobuf/types/known/emptypb"
)

type DispatcherClient interface {
	SubmitJob(ctx context.Context, job *proto.JobRequest) (*proto.SubmitJobResponse, error)
	GetStatus(ctx context.Context) (*proto.DispatcherStatus, error)
	UpdateRunnerDispatch(ctx context.Context, req *proto.UpdateRunnerDispatchRequest) (*proto.UpdateRunnerDispatchResponse, error)
}

type grpcDispatcherClient struct {
	client proto.DispatcherServiceClient
}

func NewDispatcherClient(client proto.DispatcherServiceClient) DispatcherClient {
	return grpcDispatcherClient{client: client}
}

func (c grpcDispatcherClient) SubmitJob(ctx context.Context, job *proto.JobRequest) (*proto.SubmitJobResponse, error) {
	return c.client.SubmitJob(ctx, job)
}

func (c grpcDispatcherClient) GetStatus(ctx context.Context) (*proto.DispatcherStatus, error) {
	return c.client.GetStatus(ctx, &emptypb.Empty{})
}

func (c grpcDispatcherClient) UpdateRunnerDispatch(ctx context.Context, req *proto.UpdateRunnerDispatchRequest) (*proto.UpdateRunnerDispatchResponse, error) {
	return c.client.UpdateRunnerDispatch(ctx, req)
}
