package main

import (
	"container/list"
	"errors"
	"github.com/Terry-Mao/gopush-cluster/hash"
	"net"
	"sync"
	"time"
)

const (
	Second           = int64(time.Second)
	InnerChannelType = 1
	OuterChannelType = 2
)

var (
	ErrChannelNotExist = errors.New("Channle not exist")
	ErrChannelExpired  = errors.New("Channel expired")
)

var (
	UserChannel *ChannelList
)

// The subscriber interface.
type Channel interface {
	// PushMsg push a message to the subscriber.
	PushMsg(m *Message, key string) error
	// Add a token for one subscriber
	// The request token not equal the subscriber token will return errors.
	AddToken(token string, expire int64, key string) error
	// Auth auth the access token.
	// The request token not match the subscriber token will return errors.
	AuthToken(token string, key string) error
	// AddConn add a connection for the subscriber.
	// Exceed the max number of subscribers per key will return errors.
	AddConn(conn net.Conn, key string) (*list.Element, error)
	// RemoveConn remove a connection for the  subscriber.
	RemoveConn(e *list.Element, key string) error
	// SetDeadline set the channel deadline unixnano.
	SetDeadline(d int64)
	Timeout() bool
	// Expire expire the channle and clean data.
	Close() error
}

// Channel bucket.
type ChannelBucket struct {
	Data  map[string]Channel
	mutex *sync.Mutex
}

// Channel list.
type ChannelList struct {
	Channels []*ChannelBucket
}

// Lock lock the bucket mutex.
func (c *ChannelBucket) Lock() {
	c.mutex.Lock()
}

// Unlock unlock the bucket mutex.
func (c *ChannelBucket) Unlock() {
	c.mutex.Unlock()
}

// NewChannelList create a new channel bucket set.
func NewChannelList() *ChannelList {
	l := &ChannelList{Channels: []*ChannelBucket{}}
	// split hashmap to many bucket
	Log.Debug("create %d ChannelBucket", Conf.ChannelBucket)
	for i := 0; i < Conf.ChannelBucket; i++ {
		c := &ChannelBucket{
			Data:  map[string]Channel{},
			mutex: &sync.Mutex{},
		}

		l.Channels = append(l.Channels, c)
	}

	return l
}

// Count get the bucket total channel count.
func (l *ChannelList) Count() int {
	c := 0
	for i := 0; i < Conf.ChannelBucket; i++ {
		c += len(l.Channels[i].Data)
	}

	return c
}

// bucket return a channelBucket use murmurhash3.
func (l *ChannelList) bucket(key string) *ChannelBucket {
	h := hash.NewMurmur3C()
	h.Write([]byte(key))
	idx := uint(h.Sum32()) & uint(Conf.ChannelBucket-1)
	Log.Debug("user_key:\"%s\" hit channel bucket index:%d", key, idx)
	return l.Channels[idx]
}

// New create a user channle.
func (l *ChannelList) New(key string) (Channel, error) {
	// get a channel bucket
	b := l.bucket(key)
	b.Lock()
	defer b.Unlock()

	if c, ok := b.Data[key]; ok {
		// refresh the expire time
		Log.Debug("user_key:\"%s\" refresh channel bucket expire time", key)
		c.SetDeadline(time.Now().UnixNano() + Conf.ChannelExpireSec*Second)
		ChStat.IncrAccess()
		return c, nil
	} else {
		Log.Debug("user_key:\"%s\" create a new channel", key)
		c = NewOuterChannel()
		ChStat.IncrCreate()
		b.Data[key] = c
		return c, nil
	}
}

// Get a user channel from ChannleList.
func (l *ChannelList) Get(key string) (Channel, error) {
	// get a channel bucket
	b := l.bucket(key)
	b.Lock()
	defer b.Unlock()

	if c, ok := b.Data[key]; !ok {
		if Conf.Auth == 0 {
			Log.Debug("user_key:\"%s\" create a new channel", key)
			c = NewOuterChannel()
			c.SetDeadline(time.Now().UnixNano() + Conf.ChannelExpireSec*Second)
			ChStat.IncrCreate()
			b.Data[key] = c
			return c, nil
		}

		Log.Warn("user_key:\"%s\" channle not exists", key)
		return nil, ErrChannelNotExist
	} else {
		// check expired
		if c.Timeout() {
			Log.Warn("user_key:\"%s\" channle expired", key)
			delete(b.Data, key)
			if err := c.Close(); err != nil {
				Log.Error("user_key:\"%s\" channel close failed (%s)", key, err.Error())
				return nil, err
			}

			ChStat.IncrExpire()
			return nil, ErrChannelExpired
		}

		Log.Debug("user_key:\"%s\" refresh channel bucket expire time", key)
		c.SetDeadline(time.Now().UnixNano() + Conf.ChannelExpireSec*Second)
		ChStat.IncrAccess()
		return c, nil
	}
}

// Delete a user channel from ChannleList.
func (l *ChannelList) Delete(key string) (Channel, error) {
	// get a channel bucket
	b := l.bucket(key)
	b.Lock()

	if c, ok := b.Data[key]; !ok {
		Log.Warn("user_key:\"%s\" channle not exists", key)
		b.Unlock()
		return nil, ErrChannelNotExist
	} else {
		Log.Info("user_key:\"%s\" delete channel", key)
		delete(b.Data, key)
		ChStat.IncrDelete()
		b.Unlock()
		return c, nil
	}
}
