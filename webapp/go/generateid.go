package main

import (
	"math/rand"
	"sync"
)

type dispenceId struct {
	seed int64
	sync.RWMutex
	data int64
}

func NewDispenceId() *dispenceId {
	return &dispenceId{
		seed: rand.Int63(),
		data: 0,
	}
}

func (c *dispenceId) Increment() int64 {
	c.Lock()
	defer c.Unlock()
	c.data++
	return c.data + c.seed
}

var DispenceId = NewDispenceId()
