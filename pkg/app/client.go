package app

import (
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"

	"github.com/SkycoinProject/dmsg/cipher"
	"github.com/SkycoinProject/skycoin/src/util/logging"

	"github.com/SkycoinProject/skywire-mainnet/pkg/app/appcommon"
	"github.com/SkycoinProject/skywire-mainnet/pkg/app/appnet"
	"github.com/SkycoinProject/skywire-mainnet/pkg/app/idmanager"
	"github.com/SkycoinProject/skywire-mainnet/pkg/routing"
)

var (
	// ErrVisorPKNotProvided is returned when the visor PK is not provided.
	ErrVisorPKNotProvided = errors.New("visor PK is not provided")
	// ErrVisorPKInvalid is returned when the visor PK is invalid.
	ErrVisorPKInvalid = errors.New("visor PK is invalid")
	// ErrSockFileNotProvided is returned when the sock file is not provided.
	ErrSockFileNotProvided = errors.New("sock file is not provided")
	// ErrAppKeyNotProvided is returned when the app key is not provided.
	ErrAppKeyNotProvided = errors.New("app key is not provided")
)

// ClientConfig is a configuration for `Client`.
type ClientConfig struct {
	VisorPK  cipher.PubKey
	SockFile string
	AppKey   appcommon.Key
}

// ClientConfigFromEnv creates client config from the ENV args.
func ClientConfigFromEnv() (ClientConfig, error) {
	appKey := os.Getenv(appcommon.EnvAppKey)
	if appKey == "" {
		return ClientConfig{}, ErrAppKeyNotProvided
	}

	sockFile := os.Getenv(appcommon.EnvSockFile)
	if sockFile == "" {
		return ClientConfig{}, ErrSockFileNotProvided
	}

	visorPKStr := os.Getenv(appcommon.EnvVisorPK)
	if visorPKStr == "" {
		return ClientConfig{}, ErrVisorPKNotProvided
	}

	var visorPK cipher.PubKey
	if err := visorPK.UnmarshalText([]byte(visorPKStr)); err != nil {
		return ClientConfig{}, ErrVisorPKInvalid
	}

	return ClientConfig{
		VisorPK:  visorPK,
		SockFile: sockFile,
		AppKey:   appcommon.Key(appKey),
	}, nil
}

// Client is used by skywire apps.
type Client struct {
	log     *logging.Logger
	visorPK cipher.PubKey
	rpc     RPCClient
	lm      *idmanager.Manager // contains listeners associated with their IDs
	cm      *idmanager.Manager // contains connections associated with their IDs
}

// NewClient creates a new `Client`. The `Client` needs to be provided with:
// - log: logger instance.
// - config: client configuration.
func NewClient(log *logging.Logger, config ClientConfig) (*Client, error) {
	rpcCl, err := rpc.Dial("unix", config.SockFile)
	if err != nil {
		return nil, fmt.Errorf("error connecting to the app server: %v", err)
	}

	return &Client{
		log:     log,
		visorPK: config.VisorPK,
		rpc:     NewRPCClient(rpcCl, config.AppKey),
		lm:      idmanager.New(),
		cm:      idmanager.New(),
	}, nil
}

// Dial dials the remote node using `remote`.
func (c *Client) Dial(remote appnet.Addr) (net.Conn, error) {
	connID, localPort, err := c.rpc.Dial(remote)
	if err != nil {
		return nil, err
	}

	conn := &Conn{
		id:  connID,
		rpc: c.rpc,
		local: appnet.Addr{
			Net:    remote.Net,
			PubKey: c.visorPK,
			Port:   localPort,
		},
		remote: remote,
	}

	conn.freeConnMx.Lock()

	free, err := c.cm.Add(connID, conn)

	if err != nil {
		conn.freeConnMx.Unlock()

		if err := conn.Close(); err != nil {
			c.log.WithError(err).Error("error closing conn")
		}

		return nil, err
	}

	conn.freeConn = free

	conn.freeConnMx.Unlock()

	return conn, nil
}

// Listen listens on the specified `port` for the incoming connections.
func (c *Client) Listen(n appnet.Type, port routing.Port) (net.Listener, error) {
	local := appnet.Addr{
		Net:    n,
		PubKey: c.visorPK,
		Port:   port,
	}

	lisID, err := c.rpc.Listen(local)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		log:  c.log,
		id:   lisID,
		rpc:  c.rpc,
		addr: local,
		cm:   idmanager.New(),
	}

	listener.freeLisMx.Lock()

	freeLis, err := c.lm.Add(lisID, listener)
	if err != nil {
		listener.freeLisMx.Unlock()

		if err := listener.Close(); err != nil {
			c.log.WithError(err).Error("error closing listener")
		}

		return nil, err
	}

	listener.freeLis = freeLis

	listener.freeLisMx.Unlock()

	return listener, nil
}

// Close closes client/server communication entirely. It closes all open
// listeners and connections.
func (c *Client) Close() {
	var listeners []net.Listener

	c.lm.DoRange(func(_ uint16, v interface{}) bool {
		lis, err := idmanager.AssertListener(v)
		if err != nil {
			c.log.Error(err)
			return true
		}

		listeners = append(listeners, lis)
		return true
	})

	var conns []net.Conn

	c.cm.DoRange(func(_ uint16, v interface{}) bool {
		conn, err := idmanager.AssertConn(v)
		if err != nil {
			c.log.Error(err)
			return true
		}

		conns = append(conns, conn)
		return true
	})

	for _, lis := range listeners {
		if err := lis.Close(); err != nil {
			c.log.WithError(err).Error("error closing listener")
		}
	}

	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			c.log.WithError(err).Error("error closing conn")
		}
	}
}
