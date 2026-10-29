package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

const (
	channel = "realtime:events" // the pub/sub channel every copy subscribes to
	seqKey  = "realtime:seq"    // a shared counter, so IDs mean the same thing on every copy
)

// redisBus connects this server's hub to every other copy's hub.
//
// Publishing takes the next ID from a shared counter (INCR) and PUBLISHes
// the event. Every copy, including this one, is SUBSCRIBEd and hands what
// arrives to its local hub, which sends it to its own clients.
//
// Redis pub/sub is fire-and-forget: a copy that isn't subscribed at that
// moment never sees the event. For replay after downtime, use a Redis
// Stream (XADD/XREAD) instead; chapter 8.7 explains the trade-off.
type redisBus struct {
	rdb *redis.Client
	sub *redis.PubSub
}

func newRedisBus(ctx context.Context, addr string, hub *Hub) (*redisBus, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	sub := rdb.Subscribe(ctx, channel)
	if _, err := sub.Receive(ctx); err != nil { // wait until the subscription is live
		return nil, err
	}
	go func() {
		for msg := range sub.Channel() { // ends when sub is closed
			var e Event
			if err := json.Unmarshal([]byte(msg.Payload), &e); err != nil {
				log.Printf("bad event on %s: %v", channel, err)
				continue
			}
			hub.Deliver(e)
		}
	}()
	return &redisBus{rdb: rdb, sub: sub}, nil
}

func (b *redisBus) Publish(ctx context.Context, data string) (Event, error) {
	id, err := b.rdb.Incr(ctx, seqKey).Result()
	if err != nil {
		return Event{}, err
	}
	e := Event{ID: id, Data: data}
	payload, _ := json.Marshal(e)
	return e, b.rdb.Publish(ctx, channel, payload).Err()
}

func (b *redisBus) Close() {
	_ = b.sub.Close()
	_ = b.rdb.Close()
}
