/*
Copyright 2023 The KubeService-Stack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodestats

import (
	"net/http"
	"time"
)

type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

type HttpClient struct {
	client       *http.Client
	maxPoolSize  int
	cSemaphore   chan int
	reqPerSecond int
	rateLimiter  <-chan time.Time
}

func NewHttpClient(stdClient *http.Client, maxPoolSize int, reqPerSec int) Client {
	var semaphore chan int = nil
	if maxPoolSize > 0 {
		semaphore = make(chan int, maxPoolSize) // Buffered channel to act as a semaphore
	}

	var emitter <-chan time.Time = nil
	if reqPerSec > 0 {
		emitter = time.NewTicker(time.Second / time.Duration(reqPerSec)).C // x req/s == 1s/x req (inverse)
	}

	return &HttpClient{
		client:       stdClient,
		maxPoolSize:  maxPoolSize,
		cSemaphore:   semaphore,
		reqPerSecond: reqPerSec,
		rateLimiter:  emitter,
	}
}

func (c *HttpClient) Do(req *http.Request) (*http.Response, error) {
	return c.DoPool(req)
}

func (c *HttpClient) DoPool(req *http.Request) (*http.Response, error) {
	if c.maxPoolSize > 0 {
		c.cSemaphore <- 1 // Grab a connection from our pool
		defer func() {
			<-c.cSemaphore // Defer release our connection back to the pool
		}()
	}

	if c.reqPerSecond > 0 {
		<-c.rateLimiter // Block until a signal is emitted from the rateLimiter
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &http.Response{}, err
	}

	return resp, nil
}
