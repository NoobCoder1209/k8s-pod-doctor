package doctor

import "errors"

// ErrPodNotFound means the named pod does not exist in the named namespace.
var ErrPodNotFound = errors.New("pod not found")

// ErrContainerNotReady is returned by TailLogs when the apiserver replies
// 400 BadRequest because the container is still ContainerCreating /
// PodInitializing.
var ErrContainerNotReady = errors.New("container not ready for logs")
