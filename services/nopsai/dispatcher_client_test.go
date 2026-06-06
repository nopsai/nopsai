package nopsai

import (
	"context"
	"errors"
	"testing"

	"nopsai/pkg/proto"
)

func TestFetchDispatcherStatusUsesInjectedDispatcherClient(t *testing.T) {
	dispatcher := &fakeDispatcherClient{
		status: &proto.DispatcherStatus{QueuedJobs: 7},
	}
	app := &App{dispatcher: dispatcher}

	got, err := app.fetchDispatcherStatus(context.Background())
	if err != nil {
		t.Fatalf("fetchDispatcherStatus() error = %v", err)
	}
	if got.GetQueuedJobs() != 7 {
		t.Fatalf("queued jobs = %d, want 7", got.GetQueuedJobs())
	}
	if !dispatcher.statusCalled {
		t.Fatal("dispatcher GetStatus was not called")
	}
}

type fakeDispatcherClient struct {
	status       *proto.DispatcherStatus
	statusCalled bool
}

func (f *fakeDispatcherClient) SubmitJob(ctx context.Context, job *proto.JobRequest) (*proto.SubmitJobResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDispatcherClient) GetStatus(ctx context.Context) (*proto.DispatcherStatus, error) {
	f.statusCalled = true
	return f.status, nil
}

func (f *fakeDispatcherClient) UpdateRunnerDispatch(ctx context.Context, req *proto.UpdateRunnerDispatchRequest) (*proto.UpdateRunnerDispatchResponse, error) {
	return nil, errors.New("not implemented")
}
