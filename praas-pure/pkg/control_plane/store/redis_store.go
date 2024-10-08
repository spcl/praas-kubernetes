package store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v9"
	"go.uber.org/zap"
	"praas/pkg/config"
	"praas/pkg/control_plane/scraping"
	"praas/pkg/logging"
	"praas/pkg/types"
)

const (
	// How much to wait before re-trying the initial redis ping msg
	retryWait = time.Minute

	// Adding the curly braces ensures that only the text within the braces gets hashed (forces same slot)
	nextAppIDKey = "{PRAAS}.NEXT-ID"
	appSetKey    = "{PRAAS}.APPS"

	processesField       = ".PROCESSES"
	activeProcessesField = ".ACTIVE-PROCESSES"

	processFuncCountsField = ".FUNCS"

	notificationChannelPrefix = "NOTIFY."

	// TODO(gr): The process and session creation scripts allocate numbers based on a simple counter,
	//          this might need to change

	// Script: Increment counter and save it in the tracking set
	appCreationScript = `
	local pid = redis.call('INCR', KEYS[1])
	redis.call('SADD', KEYS[2], pid)
	redis.call('PUBLISH', KEYS[3], 'sadd:' .. pid)
	return pid
	`

	// TODO: If session ID was given check if it existed before
	// Script: Check if process tracking set exists for app
	//         Add process id to tracking set and active processes set
	procCreationScript = `
	if redis.call('EXISTS', KEYS[1]) == 1 then
		local exists = redis.call('SADD', KEYS[1], ARGV[1])
        if exists ~= tonumber(ARGV[2]) then
            return -2
        end
        local active = redis.call('SADD', KEYS[2], ARGV[1])
        if active == 1 then
            redis.call('PUBLISH', KEYS[3], 'sadd:' .. ARGV[1])
        end
        return active
    else
        return -1
	end
	`

	procSelectScript = `
    if redis.call('EXISTS', KEYS[2]) == 1 then
        local minProcTable = redis.call('ZRANGE', KEYS[1], 0, ARGV[1], 'BYSCORE', 'LIMIT', 0, 1)
        if not next(minProcTable)  then
            return "EMPTY"
        end
        local minProc = minProcTable[1]
        redis.call('ZINCRBY', KEYS[1], 1, minProc)
        return minProc
    else
        return "NOAPP"
    end
    `
)

var (
	failedToGetAppIDError = errors.New("failed to allocate id for new app")
	FailedToDeleteProcess = errors.New("failed to delete process from state")
	NoSuchProcess         = errors.New("process does not exist")
	NoSuchApp             = errors.New("app does not exist")
)

type redisStore struct {
	client     redis.UniversalClient
	logger     *zap.SugaredLogger
	collector  scraping.ScraperCollection
	appCreate  *redis.Script
	procCreate *redis.Script
	procSelect *redis.Script
}

var _ StateStore = (*redisStore)(nil)

func getKeyBase(appID types.IDType) string {
	return fmt.Sprintf("{app-%d}", appID)
}

type storeKey struct{}

func WithRedisStore(ctx context.Context) context.Context {
	logger := logging.FromContext(ctx)
	collector := scraping.Get(ctx)
	cfg := config.GetPraasConfig(ctx)
	store := newRedisStore(ctx, logger, collector, cfg)
	return context.WithValue(ctx, storeKey{}, store)
}

func getRedisStore(ctx context.Context) *redisStore {
	untyped := ctx.Value(storeKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Unable to fetch redis store from context.")
	}
	return untyped.(*redisStore)
}

func newRedisStore(
	ctx context.Context,
	logger *zap.SugaredLogger,
	collector scraping.ScraperCollection,
	cfg *config.PraasConfig,
) *redisStore {
	var client redis.UniversalClient

	// Check if we can connect to redis
	available := false
	for !available {
		client = redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{cfg.RedisAddress}})
		_, err := client.Ping(context.Background()).Result()

		if err != nil {
			if isRecoverableNetOpError(logger, err) {
				// If it's an error we know we can recover from, we wait
				time.Sleep(retryWait)
			} else {
				// It's not an error we know or can recover from; die
				logger.Fatalw("Could not connect to redis", zap.Error(err), zap.Any("errorType", reflect.TypeOf(err)))
			}
		} else {
			logger.Info("Successfully connected to redis")
			available = true
		}
	}

	procCreate := redis.NewScript(procCreationScript)
	err := procCreate.Load(context.Background(), client).Err()
	if err != nil {
		logger.Fatalw("Could not load session creation script into redis", zap.Error(err))
	}

	appCreate := redis.NewScript(appCreationScript)
	err = appCreate.Load(context.Background(), client).Err()
	if err != nil {
		logger.Fatalw("Could not load process creation script into redis", zap.Error(err))
	}

	funcAlloc := redis.NewScript(procSelectScript)
	err = funcAlloc.Load(context.Background(), client).Err()
	if err != nil {
		logger.Fatalw("Could not load process selection script into redis", zap.Error(err))
	}

	store := &redisStore{
		collector:  collector,
		client:     client,
		logger:     logger,
		procCreate: procCreate,
		appCreate:  appCreate,
		procSelect: funcAlloc,
	}

	store.watchKey(ctx, appSetKey, store.handleAppAdd, store.handleAppDelete)
	if err != nil {
		logger.Errorw("Failed to set up redis watch for process tracker", zap.Error(err))
	}

	err = store.setupScrapingForExistingProcesses(ctx)

	return store
}

func isRecoverableNetOpError(logger *zap.SugaredLogger, err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		if isRecoverableDNSError(logger, opErr) {
			return true
		}

		if isRecoverableOSError(logger, opErr) {
			return true
		}

		logger.Errorw("Unrecoverable net.Op error", zap.Error(err))
	}
	return false
}

func isRecoverableOSError(logger *zap.SugaredLogger, err *net.OpError) bool {
	if osErr, ok := err.Unwrap().(*os.SyscallError); ok {
		if errNo, ok := osErr.Unwrap().(syscall.Errno); ok {
			if errors.Is(errNo, syscall.ECONNREFUSED) {
				// Connection was refused, maybe redis is just coming online
				logger.Info("Syscall Error number: Connection refused. Is Redis online?")
				return true
			} else {
				logger.Errorw("Unrecoverable Syscall Error Number", zap.Error(errNo))
				return false
			}
		}
		logger.Errorw("Unrecoverable OS Error", zap.Error(osErr))
	}
	return false
}

func isRecoverableDNSError(logger *zap.SugaredLogger, err *net.OpError) bool {
	if dnsErr, ok := err.Unwrap().(*net.DNSError); ok {
		if dnsErr.IsNotFound || dnsErr.IsTemporary {
			// The redis cluster does not exist yet, we have no option, but to wait for it to come online
			logger.Info("DNS error, could not find redis, is config correct?")
			return true
		}

		logger.Errorw("Unrecoverable DNS error", zap.Error(dnsErr))
	}

	return false
}

func (r *redisStore) NewProcess(ctx context.Context, process types.ProcessRef, swap bool) ([]string, error) {
	if process.PID == "" {
		return nil, InvalidPID
	}

	keyBase := getKeyBase(process.AppID)
	keys := []string{keyBase + processesField, keyBase + activeProcessesField, notificationChannelPrefix + keyBase + activeProcessesField}
	addCount := 1
	if swap {
		addCount = 0
	}
	pipe := r.client.Pipeline()
	// activeProcsCMD := pipe.SMembers(ctx, keyBase+activeProcessesField)
	createCMD := r.procCreate.Run(ctx, pipe, keys, process.PID, addCount)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	result, err := createCMD.Int()
	if err != nil {
		return nil, err
	}
	if result == -1 {
		return nil, AppDoesNotExist
	}
	if result == 0 {
		return nil, ProcessAlreadyActive
	}
	if result == -2 {
		return nil, NoSuchProcess
	}

	activeProcs := []string{} // activeProcsCMD.Val()
	// r.logger.Infow("Created new proc: "+process.String(), zap.Any("active-processes", activeProcs))

	return activeProcs, nil
}

func (r *redisStore) DeleteProcess(ctx context.Context, ref types.ProcessRef) error {
	keyBase := getKeyBase(ref.AppID)
	pipe := r.client.Pipeline()
	remCMD := pipe.SRem(ctx, keyBase+activeProcessesField, ref.PID)
	pipe.ZRem(ctx, keyBase+processFuncCountsField, ref.PID)
	_, err := pipe.Exec(ctx)
	if err != nil {
		r.logger.Errorw("Error while deleting process", zap.Error(err))
	}

	remCount, err := remCMD.Result()
	if err != nil {
		r.logger.Errorw("Could not delete process", zap.Error(err), zap.Any("process", ref))
		return FailedToDeleteProcess
	}

	if remCount == 0 {
		return NoSuchProcess
	}
	return nil
}

func (r *redisStore) CreateApp(ctx context.Context) (types.IDType, error) {
	keys := []string{nextAppIDKey, appSetKey, notificationChannelPrefix + appSetKey}
	result, err := r.appCreate.Run(ctx, r.client, keys).Int()
	if err != nil {
		r.logger.Errorw("Could not create new process", zap.Error(err))
		return types.InvalidAppID, failedToGetAppIDError
	}
	pid := types.IDType(result)

	// Post the pods set with a random value (empty sets are not allowed)
	// This needs to be a separate call, because the keys might not hash to the same slots
	pipe := r.client.Pipeline()
	pipe.SAdd(ctx, getKeyBase(pid)+processesField, "<empty>")
	pipe.SAdd(ctx, getKeyBase(pid)+activeProcessesField, "<empty>")
	_, err = pipe.Exec(ctx)
	if err != nil {
		// TODO(gr): Rollback
		r.logger.Errorw("Failed to set session tracker up", zap.Error(err))
		return types.InvalidAppID, err
	}

	return pid, nil
}

func (r *redisStore) DeleteApp(ctx context.Context, pid types.IDType) error {
	// // Delete process from tracker set, and notify other instances
	keyBase := getKeyBase(pid)
	pipe := r.client.Pipeline()
	sremCmd := pipe.SRem(ctx, appSetKey, pid)
	pipe.Del(ctx, keyBase+processesField, keyBase+activeProcessesField, keyBase+processFuncCountsField)
	pipe.Publish(ctx, notificationChannelPrefix+appSetKey, "srem:"+pid.String())
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		r.logger.Errorw("Failed to delete process from tracker set", zap.Error(err))
		return err // TODO(gr): Better error here?
	}

	if sremCmd.Val() == 0 {
		return AppDoesNotExist
	}

	return nil
}

func (r *redisStore) watchKey(ctx context.Context, keyName string, handleAdd, handleDelete func(string)) {
	r.logger.Debug("WatchKey: ", keyName)
	pubsub := r.client.Subscribe(ctx, notificationChannelPrefix+keyName)
	go func() {
		for msg := range pubsub.Channel(redis.WithChannelHealthCheckInterval(0)) {
			r.logger.Debugf("Received message on channel: %s, msg: %s", msg.Channel, msg.Payload)
			msgParts := strings.Split(msg.Payload, ":")
			ref := msgParts[1]
			if msgParts[0] == "sadd" {
				handleAdd(ref)
			} else if msgParts[0] == "srem" {
				handleDelete(ref)
			} else {
				r.logger.Warnf("Channel %s: unexpected message: %s ", msg.Channel, msg.Payload)
			}
		}
	}()
}

func (r *redisStore) handleAppAdd(pidRef string) {
	// r.logger.Info("Handle process add: " + pidRef)
	pid := types.IDFromString(pidRef)
	if pid == types.InvalidAppID {
		r.logger.Errorf("Could not parse pid: %s", pidRef)
		return
	}
	err := r.collector.CreateCollection(pid)
	if err != nil && !errors.Is(err, scraping.AppExists) {
		r.logger.Errorw("Failed to create collection received from redis", zap.Error(err))
	}
}

func (r *redisStore) handleAppDelete(pidRef string) {
	// Cancel watching the process pod fields
	pid := types.IDFromString(pidRef)
	if pid == types.InvalidAppID {
		r.logger.Errorf("Could not parse pid: %s", pidRef)
		return
	}

	// Delete from local scraping memory
	err := r.collector.DeleteCollection(pid)
	if err != nil {
		r.logger.Errorw("Failed to delete collection received from redis", zap.Error(err))
	}
}

func (r *redisStore) ProcessExistsAndActive(ctx context.Context, process types.ProcessRef) bool {
	key := getKeyBase(process.AppID) + activeProcessesField
	isMember, err := r.client.SIsMember(ctx, key, process.PID).Result()
	if err != nil {
		return false
	}
	return isMember
}

func (r *redisStore) setupScrapingForExistingProcesses(ctx context.Context) error {
	// Get apps
	result, err := r.client.SMembers(ctx, appSetKey).Result()
	if err != nil {
		return err
	}

	for _, pidStr := range result {
		// Setup each app
		r.handleAppAdd(pidStr)
	}

	return nil
}

func (r *redisStore) GetProcesses(ctx context.Context, appID types.IDType) ([]string, []string, error) {
	keyBase := getKeyBase(appID)
	pipe := r.client.Pipeline()
	procCMD := pipe.SMembers(ctx, keyBase+processesField)
	aProcCMD := pipe.SMembers(ctx, keyBase+activeProcessesField)
	_, err := pipe.Exec(ctx)
	return procCMD.Val(), aProcCMD.Val(), err
}

func (r *redisStore) GetProcessForFunc(
	ctx context.Context,
	appID types.IDType,
	funcLimit int,
) (string, []string, error) {
	keyBase := getKeyBase(appID)
	keys := []string{keyBase + processFuncCountsField, keyBase + processesField}
	pipe := r.client.Pipeline()
	inactiveProcsCMD := pipe.SDiff(ctx, keyBase+processesField, keyBase+activeProcessesField)
	procSelectCMD := r.procSelect.Run(ctx, pipe, keys, funcLimit-1)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return "", nil, err
	}

	r.logger.Info("Script result: ", procSelectCMD.Val())
	selectedProc := procSelectCMD.Val().(string)
	if selectedProc == "EMPTY" {
		err = NoSuchProcess
	} else if selectedProc == "NOAPP" {
		err = NoSuchApp
	}

	return selectedProc, inactiveProcsCMD.Val(), err
}

func (r *redisStore) AllocFunc(ctx context.Context, appID types.IDType, pid string) error {
	keyBase := getKeyBase(appID)
	return r.client.ZIncrBy(ctx, keyBase+processFuncCountsField, 1, pid).Err()
}

func (r *redisStore) DeAllocFunc(ctx context.Context, appID types.IDType, pid string) error {
	keyBase := getKeyBase(appID)
	return r.client.ZIncrBy(ctx, keyBase+processFuncCountsField, -1, pid).Err()
}
