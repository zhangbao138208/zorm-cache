package util

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"
)

func GenInstanceId() string {
	charList := []byte("1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	rand.Seed(time.Now().Unix())
	length := 5
	str := make([]byte, 0)
	for i := 0; i < length; i++ {
		str = append(str, charList[rand.Intn(len(charList))])
	}
	return string(str)
}

func GenPrimaryCacheKey(instanceId string, tableName string, primaryKey string) string {
	return fmt.Sprintf("%s:%s:p:%s:%s", GormCachePrefix, instanceId, tableName, primaryKey)
}

func GenPrimaryCachePrefix(instanceId string, tableName string) string {
	return GormCachePrefix + ":" + instanceId + ":p:" + tableName
}

func GenSearchCacheKey(instanceId string, tableName string, sql string, vars ...interface{}) string {
	buf := strings.Builder{}
	buf.WriteString(sql)
	for _, v := range vars {
		pv := reflect.ValueOf(v)
		if pv.Kind() == reflect.Ptr {
			buf.WriteString(fmt.Sprintf(":%v", pv.Elem()))
		} else {
			buf.WriteString(fmt.Sprintf(":%v", v))
		}
	}
	return fmt.Sprintf("%s:%s:s:%s:%s", GormCachePrefix, instanceId, tableName, buf.String())
}

const (
	roundId        = "`round_id`"
	transactionId  = "`transaction_id`"
	gameProviderId = "`game_provider_id`"
	gameTypeId     = "`game_type_id`"
	xbUid          = "`xb_uid`"
	ID             = "`id`"
)

func GenBetCacheKey(tableName string, keyMap map[string]interface{}) (string, bool) {
	var rId, tId, gpId, uid interface{}
	v, ok := keyMap[roundId]
	if ok {
		rId = v
	}
	v, ok = keyMap[transactionId]
	if ok {
		tId = v
	}
	v, ok = keyMap[gameTypeId]
	//if ok {
	//	gtId = v
	//}
	v, ok = keyMap[gameProviderId]
	if ok {
		gpId = v
	}
	v, ok = keyMap[xbUid]
	if ok {
		uid = v
	}
	if rId == nil {
		return fmt.Sprintf("%s:Bet:%v:%v:%v", tableName, uid, gpId, tId), false
	}
	return fmt.Sprintf("%s:Bet:%v:%v:%v*", tableName, uid, gpId, rId), true
}

func GenBetDetailsCacheKey(tableName string, keyMap map[string]interface{}) string {
	var id, uid interface{}
	v, ok := keyMap[ID]
	if ok {
		id = v
	}
	v, ok = keyMap[xbUid]
	if ok {
		uid = v
	}
	return fmt.Sprintf("%s:Bet:%v:%v", tableName, uid, id)
}

func GenSearchCachePrefix(instanceId string, tableName string) string {
	return GormCachePrefix + ":" + instanceId + ":s:" + tableName
}

func GenSingleFlightKey(tableName string, sql string, vars ...interface{}) string {
	buf := strings.Builder{}
	buf.WriteString(sql)
	for _, v := range vars {
		pv := reflect.ValueOf(v)
		if pv.Kind() == reflect.Ptr {
			buf.WriteString(fmt.Sprintf(":%v", pv.Elem()))
		} else {
			buf.WriteString(fmt.Sprintf(":%v", v))
		}
	}
	return fmt.Sprintf("%s:%s", tableName, buf.String())
}
