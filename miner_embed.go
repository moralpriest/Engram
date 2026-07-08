package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deroproject/derohe/astrobwt/astrobwt_fast"
	"github.com/deroproject/derohe/astrobwt/astrobwtv3"
	"github.com/deroproject/derohe/block"
	"github.com/deroproject/derohe/globals"
	"github.com/deroproject/derohe/rpc"
	"github.com/gorilla/websocket"

	"github.com/civilware/tela/logger"
)

const (
	minerStopped = iota
	minerActive
)

var (
	embeddedMiner struct {
		mission    int64 // atomic: minerActive or minerStopped
		walletAddr string
		daemonAddr string
		threads    int
	}

	job        rpc.GetBlockTemplate_Result
	jobMu      sync.RWMutex
	jobCounter int64

	minerCounter    uint64
	minerDifficulty uint64
	minerHeight     int64

	blockCounter     uint64
	miniBlockCounter uint64
	rejectedCounter  uint64
)

func daemonWorkPort() int {
	switch session.Network {
	case NETWORK_TESTNET:
		return DEFAULT_TESTNET_WORK_PORT
	case NETWORK_SIMULATOR:
		return DEFAULT_SIMULATOR_DAEMON_PORT
	default:
		return DEFAULT_WORK_PORT
	}
}

func startEmbeddedMiner() {
	if atomic.LoadInt64(&embeddedMiner.mission) == minerActive {
		logger.Printf("[Miner] Already running")
		return
	}

	// Resolve wallet address
	if embeddedMiner.walletAddr == "" {
		if engram.Disk != nil {
			embeddedMiner.walletAddr = engram.Disk.GetAddress().String()
		}
	}
	if embeddedMiner.walletAddr == "" {
		logger.Printf("[Miner] No wallet address configured")
		dmState.minerState = dmStateError
		uiDo(syncToggleStates)
		return
	}

	// Resolve daemon address (miner connects to getwork port)
	workPort := daemonWorkPort()
	embeddedMiner.daemonAddr = fmt.Sprintf("127.0.0.1:%d", workPort)
	if minerDaemonAddr != "" {
		embeddedMiner.daemonAddr = minerDaemonAddr
	}

	threads := minerThreads
	if threads < 1 {
		threads = 1
	}
	embeddedMiner.threads = threads

	// Reset counters
	minerCounter = 0
	minerDifficulty = 0
	minerHeight = 0
	blockCounter = 0
	miniBlockCounter = 0
	rejectedCounter = 0
	jobCounter = 0

	// Reset mining stats
	miningStats.mu.Lock()
	miningStats.StartTime = time.Now()
	miningStats.MiniBlocks = 0
	miningStats.Blocks = 0
	miningStats.Rejected = 0
	miningStats.CurrentHashrate = 0
	miningStats.NetHashrate = 0
	miningStats.LastRewardTime = time.Time{}
	miningStats.SpeedStr = ""
	miningStats.NetHashStr = ""
	miningStats.mu.Unlock()

	atomic.StoreInt64(&embeddedMiner.mission, minerActive)
	dmState.minerState = dmStateRunning
	logger.Printf("[Miner] Starting embedded miner: %d threads, wallet=%s, daemon=%s", threads, embeddedMiner.walletAddr, embeddedMiner.daemonAddr)

	go getworkEmbedded()
	for i := 0; i < threads; i++ {
		go mineblockEmbedded(i)
	}
	go minerStatsLoop()

	uiDo(syncToggleStates)
}

func stopEmbeddedMiner() {
	atomic.StoreInt64(&embeddedMiner.mission, minerStopped)
	dmState.minerState = dmStateStopped
	logger.Printf("[Miner] Stopped")
	uiDo(syncToggleStates)
}

func minerIsActive() bool {
	return atomic.LoadInt64(&embeddedMiner.mission) == minerActive
}

var (
	wsConn   *websocket.Conn
	wsConnMu sync.Mutex
)

func getworkEmbedded() {
	walletAddr := embeddedMiner.walletAddr
	u := url.URL{Scheme: "wss", Host: embeddedMiner.daemonAddr, Path: "/ws/" + walletAddr}
	logger.Printf("[Miner] Connecting to %s", u.String())

	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	for minerIsActive() {
		wsConnMu.Lock()
		if wsConn != nil {
			wsConn.Close()
			wsConn = nil
		}
		wsConnMu.Unlock()

		conn, _, err := dialer.Dial(u.String(), nil)
		if err != nil {
			logger.Printf("[Miner] Connection error: %v, retrying in 10s", err)
			time.Sleep(10 * time.Second)
			continue
		}

		wsConnMu.Lock()
		wsConn = conn
		wsConnMu.Unlock()
		logger.Printf("[Miner] Connected to daemon work endpoint")

		for minerIsActive() {
			var result rpc.GetBlockTemplate_Result
			if err := conn.ReadJSON(&result); err != nil {
				logger.Printf("[Miner] Read error: %v, reconnecting", err)
				break
			}

			jobMu.Lock()
			job = result
			jobCounter++
			jobMu.Unlock()

			if job.LastError != "" {
				logger.Printf("[Miner] Job error: %s", job.LastError)
			}

			blockCounter = job.Blocks
			miniBlockCounter = job.MiniBlocks
			rejectedCounter = job.Rejected
			minerHeight = int64(job.Height)
			minerDifficulty = job.Difficultyuint64
		}
	}

	wsConnMu.Lock()
	if wsConn != nil {
		wsConn.Close()
		wsConn = nil
	}
	wsConnMu.Unlock()
}

func mineblockEmbedded(tid int) {
	var diff big.Int
	var work [block.MINIBLOCK_SIZE]byte
	var random_buf [12]byte

	rand.Read(random_buf[:])
	scratch := astrobwt_fast.Pool.Get().(*astrobwt_fast.ScratchData)

	time.Sleep(5 * time.Second)

	nonce_buf := work[block.MINIBLOCK_SIZE-5:]
	runtime.LockOSThread()
	threadaffinity()

	var localJobCounter int64
	i := uint32(0)

	for minerIsActive() {
		jobMu.RLock()
		myjob := job
		localJobCounter = jobCounter
		jobMu.RUnlock()

		if myjob.Blockhashing_blob == "" || localJobCounter == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		n, err := hex.Decode(work[:], []byte(myjob.Blockhashing_blob))
		if err != nil || n != block.MINIBLOCK_SIZE {
			logger.Printf("[Miner] Thread %d: blockwork decode error: %v (n=%d)", tid, err, n)
			time.Sleep(time.Second)
			continue
		}

		height := binary.BigEndian.Uint64(work[0:]) & 0x000000ffffffffff

		copy(work[block.MINIBLOCK_SIZE-12:], random_buf[:])
		work[block.MINIBLOCK_SIZE-1] = byte(tid)

		diff.SetString(myjob.Difficulty, 10)

		if work[0]&0xf != 1 {
			logger.Printf("[Miner] Thread %d: unknown block version %d", tid, work[0]&0x1f)
			time.Sleep(time.Second)
			continue
		}

		if int64(height) < globals.Config.MAJOR_HF2_HEIGHT {
			for localJobCounter == jobCounter && minerIsActive() {
				i++
				binary.BigEndian.PutUint32(nonce_buf, i)
				powhash := astrobwt_fast.POW_optimized(work[:], scratch)
				atomic.AddUint64(&minerCounter, 1)

				if checkPowHashBig(powhash, &diff) {
					logger.Printf("[Miner] Found miniblock (astrobwt_fast)! difficulty=%s height=%d", myjob.Difficulty, myjob.Height)
					func() {
						defer globals.Recover(1)
						wsConnMu.Lock()
						defer wsConnMu.Unlock()
						if wsConn != nil && job.JobID == myjob.JobID {
							err := wsConn.WriteJSON(rpc.SubmitBlock_Params{
								JobID:                 myjob.JobID,
								MiniBlockhashing_blob: fmt.Sprintf("%x", work[:]),
							})
							if err != nil {
								logger.Printf("[Miner] Submit error: %v", err)
							}
						}
					}()
					miningStats.mu.Lock()
					miningStats.LastRewardTime = time.Now()
					miningStats.MiniBlocks++
					miningStats.mu.Unlock()
					time.Sleep(500 * time.Millisecond)
				}
			}
		} else {
			for localJobCounter == jobCounter && minerIsActive() {
				i++
				binary.BigEndian.PutUint32(nonce_buf, i)
				powhash := astrobwtv3.AstroBWTv3(work[:])
				atomic.AddUint64(&minerCounter, 1)

				if checkPowHashBig(powhash, &diff) {
					logger.Printf("[Miner] Found miniblock (astrobwtv3)! difficulty=%s height=%d", myjob.Difficulty, myjob.Height)
					func() {
						defer globals.Recover(1)
						wsConnMu.Lock()
						defer wsConnMu.Unlock()
						if wsConn != nil && job.JobID == myjob.JobID {
							err := wsConn.WriteJSON(rpc.SubmitBlock_Params{
								JobID:                 myjob.JobID,
								MiniBlockhashing_blob: fmt.Sprintf("%x", work[:]),
							})
							if err != nil {
								logger.Printf("[Miner] Submit error: %v", err)
							}
						}
					}()
					miningStats.mu.Lock()
					miningStats.LastRewardTime = time.Now()
					miningStats.MiniBlocks++
					miningStats.mu.Unlock()
					time.Sleep(500 * time.Millisecond)
				}
			}
		}
		runtime.Gosched()
	}
}

func minerStatsLoop() {
	var lastCounter uint64
	var lastCounterTime = time.Now()

	for minerIsActive() {
		time.Sleep(2 * time.Second)

		currentCounter := atomic.LoadUint64(&minerCounter)
		now := time.Now()

		miningStats.mu.Lock()

		// Compute hashrate
		if currentCounter > lastCounter {
			elapsed := now.Sub(lastCounterTime).Seconds()
			if elapsed > 0 {
				speed := float64(currentCounter-lastCounter) / elapsed
				miningStats.CurrentHashrate = speed
				switch {
				case speed > 1000000:
					miningStats.SpeedStr = fmt.Sprintf("%.3f MH/s", speed/1000000.0)
				case speed > 1000:
					miningStats.SpeedStr = fmt.Sprintf("%.3f KH/s", speed/1000.0)
				default:
					miningStats.SpeedStr = fmt.Sprintf("%.0f H/s", speed)
				}
			}
		}

	// Network hashrate from difficulty (job.Difficulty)
	if minerDifficulty > 0 {
		netHash := float64(minerDifficulty) / 1.8
		if netHash > 0 {
			miningStats.NetHashrate = netHash
			switch {
			case netHash > 1e12:
				miningStats.NetHashStr = fmt.Sprintf("%.3f TH/s", netHash/1e12)
			case netHash > 1e9:
				miningStats.NetHashStr = fmt.Sprintf("%.3f GH/s", netHash/1e9)
			case netHash > 1e6:
				miningStats.NetHashStr = fmt.Sprintf("%.3f MH/s", netHash/1e6)
			case netHash > 1000:
				miningStats.NetHashStr = fmt.Sprintf("%.3f KH/s", netHash/1000)
			default:
				miningStats.NetHashStr = fmt.Sprintf("%.0f H/s", netHash)
			}
		}
	}

		// Rejected shares from daemon's getwork
		miningStats.Rejected = int64(atomic.LoadUint64(&rejectedCounter))

		miningStats.mu.Unlock()

		lastCounter = currentCounter
		lastCounterTime = now

		uiDo(func() {
			updateMinerStatsUI()
		})
	}
}
