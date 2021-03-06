package main

import (
	"encoding/json"
	myrpc "github.com/Terry-Mao/gopush-cluster/rpc"
	"net"
	"net/rpc"
	"time"
)

// Public error code
const (
	OK          = 0
	ParamErr    = 65534
	InternalErr = 65535
)

// The Message struct
type Message struct {
	// Message
	Msg string `json:"msg"`
	// Message expired unixnano
	Expire int64 `json:"expire"`
	// Message id
	MsgID int64 `json:"mid"`
}

// RPC For receive offline messages
type MessageRPC struct {
	DelChan chan *DelMessageInfo
}

// StartRPC start accept rpc call
func StartRPC() error {
	msg := &MessageRPC{make(chan *DelMessageInfo, 10000)}
	rpc.Register(msg)

	// Start a routine for delete message
	go msg.DelProc()

	l, err := net.Listen("tcp", Conf.Addr)
	if err != nil {
		Log.Error("net.Listen(\"tcp\", \"%s\") failed (%v)", Conf.Addr, err)
		return err
	}

	defer func() {
		if err := l.Close(); err != nil {
			Log.Error("listener.Close() failed (%v)", err)
		}
	}()

	Log.Info("start listen admin addr:%s", Conf.Addr)
	rpc.Accept(l)
	return nil
}

// Store offline message interface
func (r *MessageRPC) Save(m *myrpc.MessageSaveArgs, ret *int) error {
	if m == nil || m.MsgID < 0 {
		*ret = ParamErr
		return nil
	}

	// Json.Marshal and save the message
	recordMsg := Message{Msg: m.Msg, Expire: m.Expire, MsgID: m.MsgID}
	message, _ := json.Marshal(recordMsg)
	if err := SaveMessage(m.Key, string(message), m.MsgID); err != nil {
		Log.Error("save message error (%v)", err)
		*ret = InternalErr
		return nil
	}

	*ret = OK
	return nil
}

// Get offline message interface
func (r *MessageRPC) Get(m *myrpc.MessageGetArgs, rw *myrpc.MessageGetResp) error {
	Log.Debug("request data %v", *m)
	// Get all of offline messages which larger than MsgID
	msgs, err := GetMessages(m.Key, m.MsgID)
	if err != nil {
		Log.Error("get messages error (%v)", err)
		rw.Ret = InternalErr
		return nil
	}

	numMsg := len(msgs)
	if len(msgs) == 0 {
		rw.Ret = OK
		return nil
	}

	var (
		data    []string
		delMsgs []string
		msg     = &Message{}
	)

	// Checkout expired offline messages
	for i := 0; i < numMsg; i++ {
		if err := json.Unmarshal([]byte(msgs[i]), &msg); err != nil {
			Log.Error("internal message:%s error (%v)", msgs[i], err)
			rw.Ret = InternalErr
			return nil
		}

		if time.Now().UnixNano() > msg.Expire {
			delMsgs = append(delMsgs, msgs[i])
			continue
		}

		data = append(data, msgs[i])
	}

	// Send to delete message process
	if len(delMsgs) != 0 {
		r.DelChan <- &DelMessageInfo{Key: m.Key, Msgs: delMsgs}
	}

	Log.Debug("response data %v", *rw)
	rw.Ret = OK
	rw.Msgs = data
	return nil
}

// Server Ping interface
func (r *MessageRPC) Ping(p int, ret *int) error {
	*ret = OK
	return nil
}

// Asynchronous delete message
func (r *MessageRPC) DelProc() {
	for {
		info := <-r.DelChan
		if err := DelMessages(info); err != nil {
			Log.Error("DelMessages(key:%s,Msgs:%v) failed (%v)", info.Key, info.Msgs, err)
		}
	}
}
