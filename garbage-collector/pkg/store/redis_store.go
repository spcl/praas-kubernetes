package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v9"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/logging"
	"praas/pkg/resources"
)

const (
	nameField            = ".NAME"
	nsField              = ".NS"
	podsField            = ".PODS"
	processesField       = ".PROCESSES"
	activeProcessesField = ".ACTIVE-PROCESSES"
	reuseField           = ".REUSED"
	processIDField       = ".AppID"
	lockField            = ".LOCK"

	nextAppIDField = "PRAAS.NEXT-ID"
)

type redisStore struct {
	client        redis.UniversalClient
	logger        *zap.SugaredLogger
	processDelete *redis.Script
	processReuse  *redis.Script
	locks         sync.Map // map[int64]*sync.Mutex
	processCreate *redis.Script
}

var _ StateStore = (*redisStore)(nil)

func getKeyBase(appID int64) string {
	return fmt.Sprintf("{app-%d}", appID)
}

type storeKey struct{}

func WithRedisStore(ctx context.Context) context.Context {
	logger := logging.FromContext(ctx)
	addr := resources.GetPraasConfig(ctx).RedisAddress
	store := newRedisStore(ctx, logger, addr)
	return context.WithValue(ctx, storeKey{}, store)
}

func getRedisStore(ctx context.Context) *redisStore {
	untyped := ctx.Value(storeKey{})
	if untyped == nil {
		logging.FromContext(ctx).Panic("Unable to fetch redis store from context.")
	}
	return untyped.(*redisStore)
}

func newRedisStore(ctx context.Context, logger *zap.SugaredLogger, redisAddr string) *redisStore {
	client := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{redisAddr}})

	// Check if we can connect to reddis
	_, err := client.Ping(ctx).Result()
	if err != nil {
		logger.Fatalw("Could not connect to redis", zap.Error(err))
	}

	// Script: Add pid to processes (check if it should have been there)
	//         If check fails return with error, otherwise add to active processes
	processCreateScript := redis.NewScript(
		`
    if redis.call('SADD', KEYS[1], ARGV[1]) ~= tonumber(ARGV[2]) then
        return -1
    end
    return redis.call('SADD', KEYS[2], ARGV[1])
`,
	)
	result, err := processCreateScript.Load(ctx, client).Result()
	if err != nil {
		logger.Fatalw("Could not load session create script ("+result+")", zap.Error(err))
	}

	// Script: if reuse field does not exist delete from active sessions then delete from pods
	processDeleteScript := redis.NewScript(
		`
	if redis.call('EXISTS', KEYS[1]) == 0 then 
        redis.call('SREM', KEYS[2], ARGV[1])
		return redis.call('SREM', KEYS[3], ARGV[2])
	else
		return -1
	end
	`,
	)
	result, err = processDeleteScript.Load(ctx, client).Result()
	if err != nil {
		logger.Fatalw("Could not load session delete script ("+result+")", zap.Error(err))
	}

	// Script: if pod is part of this app and we can set reuse field (if it does not exist),
	// then delete old process from active processes and add new process to it
	processReuseScript := redis.NewScript(
		`
    local exists = redis.call('SADD', KEYS[3], ARGV[3])
    if exists ~= tonumber(ARGV[4]) then
        return 'NOMATCH'
    end
	if redis.call('SISMEMBER', KEYS[1], ARGV[1]) == 1 then
        local canReuse = redis.call('SET', KEYS[2], '', 'EX', '5', 'NX')
        if canReuse ~= nil then
            redis.call('SREM', KEYS[4], ARGV[2])
            redis.call('SADD', KEYS[4], ARGV[3])
        end
		return canReuse 
	else
		return 'NOTMEMBER'
	end
	`,
	)
	result, err = processReuseScript.Load(ctx, client).Result()
	if err != nil {
		logger.Fatalw("Could not load session delete script ("+result+")", zap.Error(err))
	}

	return &redisStore{
		client:        client,
		logger:        logger,
		processCreate: processCreateScript,
		processDelete: processDeleteScript,
		processReuse:  processReuseScript,
	}
}

func (r *redisStore) StoreApp(ctx context.Context, app *PraasApp) error {
	pidStr := fmt.Sprint(app.ID)
	keyBase := getKeyBase(app.ID)
	namespacedName := types.NamespacedName{
		Namespace: app.ServiceNamespace,
		Name:      app.ServiceName,
	}.String()

	// Issue all commands
	pipe := r.client.Pipeline()
	pipe.Set(ctx, keyBase+nameField, app.ServiceName, 0)
	pipe.Set(ctx, keyBase+nsField, app.ServiceNamespace, 0)
	pipe.Set(ctx, namespacedName, pidStr, 0)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisStore) DeleteApp(ctx context.Context, app *PraasApp) error {
	keyBase := getKeyBase(app.ID)
	namespacedName := types.NamespacedName{
		Namespace: app.ServiceNamespace,
		Name:      app.ServiceName,
	}.String()

	// Issue all commands
	pipe := r.client.Pipeline()
	keyCmd := pipe.Del(
		ctx,
		keyBase+nameField,
		keyBase+nsField,
		keyBase+podsField,
		keyBase+processesField,
		keyBase+activeProcessesField,
	)
	nameCmd := pipe.Del(ctx, namespacedName)
	_, err := pipe.Exec(ctx)

	// Check all commands for errors
	if keyCmd.Err() != nil {
		return keyCmd.Err()
	} else if nameCmd.Err() != nil {
		return nameCmd.Err()
	}
	return err
}

func (r *redisStore) GetApp(ctx context.Context, appID int64) (*PraasApp, error) {
	keyBase := getKeyBase(appID)
	pipe := r.client.Pipeline()
	nameCmd := pipe.Get(ctx, keyBase+nameField)
	nsCmd := pipe.Get(ctx, keyBase+nsField)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return &PraasApp{
		ID:               appID,
		ServiceNamespace: nsCmd.Val(),
		ServiceName:      nameCmd.Val(),
	}, nil
}

func (r *redisStore) GetAppByNamespacedName(ctx context.Context, name types.NamespacedName) (*PraasApp, error) {
	id, err := getIntFromCmd(r.client.Get(ctx, name.String()))
	if err != nil {
		return nil, err
	}
	return &PraasApp{
		ID:               id,
		ServiceNamespace: name.Namespace,
		ServiceName:      name.Name,
	}, err
}

func (r *redisStore) GetDeletablePods(
	ctx context.Context,
	app *PraasApp,
	names []string,
) (found, notFound []string, err error) {
	keyBase := getKeyBase(app.ID)
	var reusedKeys []string
	for _, name := range names {
		reusedKeys = append(reusedKeys, keyBase+name+reuseField)
	}
	pipe := r.client.Pipeline()
	isMemberCMD := pipe.SMIsMember(ctx, keyBase+podsField, names)
	cmds := make([]*redis.IntCmd, len(names))
	for idx, key := range reusedKeys {
		cmds[idx] = pipe.Exists(ctx, key)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, nil, err
	}
	isMember := isMemberCMD.Val()
	for idx, name := range names {
		if isMember[idx] && cmds[idx].Val() == 0 {
			found = append(found, name)
		} else {
			notFound = append(notFound, name)
		}
	}
	return found, notFound, err
}

func (r *redisStore) CreateProcess(ctx context.Context, app *PraasApp, pid string, swap bool) error {
	keyBase := getKeyBase(app.ID)
	addCount := 1
	if swap {
		addCount = 0
	}
	keys := []string{keyBase + processesField, keyBase + activeProcessesField}
	result, err := r.processCreate.Run(ctx, r.client, keys, pid, addCount).Int()
	if err != nil {
		r.logger.Errorw("Failed to create session", zap.Error(err))
	}

	if result == 0 {
		return sessionAlreadyActiveError
	}
	if result == -1 {
		r.logger.Info("Return error")
		return ProcessNotFound
	}
	return nil
}

func (r *redisStore) MapPodToProcess(ctx context.Context, app *PraasApp, podName, pid string) error {
	keyBase := getKeyBase(app.ID)
	pipe := r.client.Pipeline()
	pipe.SAdd(ctx, keyBase+podsField, podName)
	pipe.Set(ctx, podName+processIDField, pid, 0)
	_, err := pipe.Exec(ctx)
	if err != nil {
		r.logger.Errorw("Failed to create session", zap.Error(err))
	}
	return err
}

func (r *redisStore) DeleteProcess(ctx context.Context, app *PraasApp, podName string) error {
	keyBase := getKeyBase(app.ID)
	sessionID, err := r.client.Get(ctx, podName+processIDField).Result()
	if err != nil {
		return err
	}

	keys := []string{keyBase + podName + reuseField, keyBase + activeProcessesField, keyBase + podsField}
	delCount, err := r.processDelete.Run(ctx, r.client, keys, sessionID, podName).Int()
	if err != nil {
		return err
	}
	if delCount != 1 {
		return fmt.Errorf("cannot delete session %v as it does not exist (%d)", podName, delCount)
	}

	// Delete the rest
	err = r.client.Del(ctx, podName+processIDField).Err()
	if err != nil {
		r.logger.Warnw("Could not delete tracking fields for a session", zap.Error(err))
	}
	return nil
}

func (r *redisStore) GetNextAppID(ctx context.Context) int64 {
	pid, err := r.client.Incr(ctx, nextAppIDField).Result()
	if err != nil {
		r.logger.Fatalw("Failed to get next pid", zap.Error(err))
	}
	return pid
}

func (r *redisStore) LockApp(ctx context.Context, app *PraasApp) (isLocked bool, err error) {
	// First lock the local lock
	var lock *sync.Mutex
	untyped, exists := r.locks.Load(app.ID) // r.locks[app.ID]
	if !exists {
		lock = &sync.Mutex{}
		r.locks.Store(app.ID, lock)
		// r.locks[app.ID] = lock
	} else {
		lock = untyped.(*sync.Mutex)
	}

	lock.Lock()
	defer func() {
		if isLocked == false {
			lock.Unlock()
		}
	}()

	// Lock app in redis
	lockKey := getKeyBase(app.ID) + lockField
	gotKey := false

	// Get an expiration date for the lock
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(time.Minute)
	}

	// Try to get lock until success
	for !gotKey {
		ok, err := r.client.SetNX(ctx, lockKey, true, deadline.Sub(time.Now())).Result()
		r.logger.Debugf("Managed to get lock: %t", ok)
		if err != nil {
			r.logger.Errorw("Failed to try to get lock", zap.Error(err))
			return false, err
		} else if ok {
			gotKey = true
		} else {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return true, nil
}

func (r *redisStore) UnLockApp(ctx context.Context, app *PraasApp) {
	err := r.client.Del(ctx, getKeyBase(app.ID)+lockField).Err()
	if err != nil {
		r.logger.Errorw("Failed to unlock app", zap.Error(err))
	}

	// Release local lock
	var lock *sync.Mutex
	untyped, exists := r.locks.Load(app.ID)
	if !exists {
		r.logger.Fatalf("Lock does not exist for app %d", app.ID)
		return
	} else {
		lock = untyped.(*sync.Mutex)
	}
	lock.Unlock()
}

func (r *redisStore) GetCurrentScale(ctx context.Context, app *PraasApp) int32 {
	keyBase := getKeyBase(app.ID)
	val, err := r.client.SCard(ctx, keyBase+podsField).Result()
	// val, err := getIntFromCmd(r.client.Get(ctx, keyBase+scaleField))
	if err != nil {
		r.logger.Fatalw("Failed to get scale from redis", zap.Error(err))
	}
	r.logger.Debug("Current scale: ", val)
	return int32(val)
}

func getIntFromCmd(cmd *redis.StringCmd) (int64, error) {
	cmdRes, err := cmd.Result()
	if err != nil {
		return -1, err
	}
	return strconv.ParseInt(cmdRes, 10, 32)
}

func (r *redisStore) MarkAsReused(ctx context.Context, app *PraasApp, podName string, pid string, swap bool) error {
	oldPIDCmd := r.client.Get(ctx, podName+processIDField)
	err := oldPIDCmd.Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		r.logger.Errorw("Failed to set payload and session id when reusing pod", zap.Error(err))
		return err
	}

	addCount := 1
	if swap {
		addCount = 0
	}
	keyBase := getKeyBase(app.ID)
	keys := []string{keyBase + podsField, keyBase + podName + reuseField, keyBase + processesField, keyBase + activeProcessesField}
	result, err := r.processReuse.Run(ctx, r.client, keys, podName, oldPIDCmd.Val(), pid, addCount).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return NotReusableAnymoreError
		}

		r.logger.Errorw("Failed to run pod reuse script", zap.Error(err))
		return err
	}
	if result == "NOTMEMBER" {
		return NotReusableAnymoreError
	}
	if result == "NOMATCH" {
		return ProcessNotFound
	}
	if result != "OK" {
		return fmt.Errorf("can't reuse pod %s (%v)", podName, result)
	}

	r.client.Set(ctx, podName+processIDField, pid, 0)

	return nil
}
