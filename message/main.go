package main

import (
	"flag"
	"github.com/Terry-Mao/gopush-cluster/log"
	"os"
	"runtime"
)

var (
	Log *log.Logger
)

func main() {
	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())

	// Load config
	InitConfig()
	flag.Parse()
	Conf, err = NewConfig(ConfFile)
	if err != nil {
		Log.Error("NewConfig(\"ConfigPath\":%s) failed(%v)", ConfFile, err)
		os.Exit(-1)
	}

	// Load log
	Log, err = log.New(Conf.LogPath, Conf.LogLevel)
	if err != nil {
		Log.Error("log.New(\"LogPath\":%s) failed(%v)", Conf.LogPath, err)
		os.Exit(-1)
	}

	// Initialize redis
	InitRedis()

	// Start rpc
	if err := StartRPC(); err != nil {
		Log.Error("StartRPC() failed (%v)", err)
		os.Exit(-1)
	}
}
