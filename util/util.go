package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func GenPSK(length int) ([]byte, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("random bytes generation failed: %w", err)
	}

	return b, nil
}

func GenPSKHex(length int) (string, error) {
	b, err := GenPSK(length)
	if err != nil {
		return "", fmt.Errorf("can't generate hex key: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func PSKFromHex(input string) ([]byte, error) {
	return hex.DecodeString(input)
}

func isTimeout(err error) bool {
	if timeoutErr, ok := err.(interface {
		Timeout() bool
	}); ok {
		return timeoutErr.Timeout()
	}
	return false
}

func isTemporary(err error) bool {
	if timeoutErr, ok := err.(interface {
		Temporary() bool
	}); ok {
		return timeoutErr.Temporary()
	}
	return false
}

const (
	MaxPktBuf = 65536
)

func PairConn(left, right net.Conn, idleTimeout time.Duration) {
	var lsn atomic.Int32
	var wg sync.WaitGroup

	copier := func(dst, src net.Conn) {
		defer wg.Done()
		defer dst.Close()
		buf := make([]byte, MaxPktBuf)
		for {
			oldLSN := lsn.Load()

			if err := src.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
				log.Printf("can't update deadline for connection: %v", err)
				break
			}

			n, err := src.Read(buf)
			if err != nil {
				if isTimeout(err) {
					// hit read deadline
					if oldLSN != lsn.Load() {
						// not stale conn
						continue
					} else {
						log.Printf("dropping stale connection %s <=> %s", src.LocalAddr(), src.RemoteAddr())
					}
				} else {
					// any other error
					if isTemporary(err) {
						log.Printf("ignoring temporary error during read from %s: %v", src.RemoteAddr(), err)
						continue
					}
					log.Printf("read from %s error: %v", src.RemoteAddr(), err)
				}
				break
			}

			lsn.Add(1)

			_, err = dst.Write(buf[:n])
			if err != nil {
				log.Printf("write to %s error: %v", dst.RemoteAddr(), err)
				break
			}
		}
	}

	wg.Add(2)
	go copier(left, right)
	go copier(right, left)
	wg.Wait()
}
