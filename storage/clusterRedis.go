package storage

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.skig.tech/zero-core/zorm-cache/util"
)

var _ DataStorage = &ClusterRedis{}

type ClusterRedisStoreConfig struct {
	KeyPrefix string // key prefix will be random if not set

	Client  *redis.ClusterClient // if Client is not nil, Options will be ignored
	Options *redis.ClusterOptions
}

func NewClusterRedis(config ...*ClusterRedisStoreConfig) *ClusterRedis {
	if len(config) == 0 {
		panic("redis config is required")
	}
	if config[0].KeyPrefix == "" {
		config[0].KeyPrefix = util.GormCachePrefix + ":" + util.GenInstanceId()
	}
	r := &ClusterRedis{
		keyPrefix: config[0].KeyPrefix,
	}
	if config[0].Client != nil {
		r.client = config[0].Client
		return r
	}
	r.client = redis.NewClusterClient(config[0].Options)
	return r
}

type ClusterRedis struct {
	client    *redis.ClusterClient
	ttl       int64
	logger    util.LoggerInterface
	keyPrefix string

	batchExistSha string
	cleanCacheSha string

	once sync.Once
}

func (r *ClusterRedis) Init(conf *Config) error {
	var err error
	r.once.Do(func() {
		r.ttl = conf.TTL
		r.logger = conf.Logger
		r.logger.SetIsDebug(conf.Debug)
		err = r.initScripts()
	})
	return err
}

func (r *ClusterRedis) initScripts() error {
	batchKeyExistScript := `
		for idx, val in pairs(KEYS) do
			local exists = redis.call('EXISTS', val)
			if exists == 0 then
				return 0
			end
		end
		return 1`

	cleanCacheScript := `
		local keys = redis.call('keys', ARGV[1])
		for i=1,#keys,5000 do 
			redis.call('del', 'defaultKey', unpack(keys, i, math.min(i+4999, #keys)))
		end
		return 1`

	result := r.client.ScriptLoad(context.Background(), batchKeyExistScript)
	if result.Err() != nil {
		r.logger.CtxError(context.Background(), "[initScripts] init script 1 error: %v", result.Err())
		return result.Err()
	}
	r.batchExistSha = result.Val()
	r.logger.CtxInfo(context.Background(), "[initScripts] init batch exist script sha1: %s", r.batchExistSha)

	result = r.client.ScriptLoad(context.Background(), cleanCacheScript)
	if result.Err() != nil {
		r.logger.CtxError(context.Background(), "[initScripts] init script 2 error: %v", result.Err())
		return result.Err()
	}
	r.cleanCacheSha = result.Val()
	r.logger.CtxInfo(context.Background(), "[initScripts] init clean cache script sha1: %s", r.cleanCacheSha)
	return nil
}

func (r *ClusterRedis) CleanCache(ctx context.Context) error {
	result := r.client.EvalSha(ctx, r.cleanCacheSha, []string{"0"}, r.keyPrefix+":*")
	if result.Err() != nil {
		r.logger.CtxError(ctx, "[CleanCache] clean cache error: %v", result.Err())
		return result.Err()
	}
	return nil
}

func (r *ClusterRedis) BatchKeyExist(ctx context.Context, keys []string) (bool, error) {
	result := r.client.EvalSha(ctx, r.batchExistSha, keys)
	if result.Err() != nil {
		r.logger.CtxError(ctx, "[BatchKeyExist] eval script error: %v", result.Err())
		return false, result.Err()
	}
	return result.Bool()
}

func (r *ClusterRedis) KeyExists(ctx context.Context, key string) (bool, error) {
	result := r.client.Exists(ctx, key)
	if result.Err() != nil {
		r.logger.CtxError(ctx, "[KeyExists] exists error: %v", result.Err())
		return false, result.Err()
	}
	if result.Val() == 1 {
		return true, nil
	}
	return false, nil
}

func (r *ClusterRedis) GetValue(ctx context.Context, key string) (data string, err error) {
	data, err = r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		err = ErrCacheNotFound
	}
	return
}

func (r *ClusterRedis) BatchGetValues(ctx context.Context, keys []string) ([]string, error) {
	result := r.client.MGet(ctx, keys...)
	if result.Err() != nil {
		r.logger.CtxError(ctx, "[BatchGetValues] mget error: %v", result.Err())
		return nil, result.Err()
	}
	slice := result.Val()
	strs := make([]string, 0, len(slice))
	for _, obj := range slice {
		if obj != nil {
			strs = append(strs, obj.(string))
		}
	}
	return strs, nil
}

func (r *ClusterRedis) DeleteKeysWithPrefix(ctx context.Context, keyPrefix string) error {
	result := r.client.EvalSha(ctx, r.cleanCacheSha, []string{"0"}, keyPrefix+":*")
	return result.Err()
}

func (r *ClusterRedis) DeleteKey(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *ClusterRedis) BatchDeleteKeys(ctx context.Context, keys []string) error {
	return r.client.Del(ctx, keys...).Err()
}

func (r *ClusterRedis) BatchSetKeys(ctx context.Context, kvs []util.Kv) error {
	if r.ttl == 0 {
		spreads := make([]interface{}, 0, len(kvs))
		for _, kv := range kvs {
			spreads = append(spreads, kv.Key)
			spreads = append(spreads, kv.Value)
		}
		return r.client.MSet(ctx, spreads...).Err()
	}
	_, err := r.client.Pipelined(ctx, func(pipeliner redis.Pipeliner) error {
		for _, kv := range kvs {
			result := pipeliner.Set(ctx, kv.Key, kv.Value, time.Duration(util.RandFloatingInt64(r.ttl))*time.Millisecond)
			if result.Err() != nil {
				r.logger.CtxError(ctx, "[BatchSetKeys] set key %s error: %v", kv.Key, result.Err())
				return result.Err()
			}
		}
		return nil
	})
	return err
}

func (r *ClusterRedis) SetKey(ctx context.Context, kv util.Kv) error {
	return r.client.Set(ctx, kv.Key, kv.Value, time.Duration(util.RandFloatingInt64(r.ttl))*time.Millisecond).Err()
}

func (r *ClusterRedis) GetKey(ctx context.Context, key string) (string, error) {
	cmd := r.client.Keys(ctx, key)
	if cmd.Err() != nil {
		return "", cmd.Err()
	}
	if len(cmd.Val()) > 0 {
		return cmd.Val()[len(cmd.Val())-1], nil
	}
	return "", nil
}
