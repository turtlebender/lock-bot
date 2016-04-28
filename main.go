package main

import (
	"fmt"
	"os"

	"strings"

	"github.com/gin-gonic/gin"

	"net/url"
	"time"

	"github.com/garyburd/redigo/redis"
)

const lockPrefix = "locker-"
const day = 24 * 60

var (
	redisPool *redis.Pool
)

func init() {
}

func newRedisPool(server string, password string) *redis.Pool {
	delegate := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			if _, err := c.Do("AUTH", password); err != nil {
				c.Close()
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	return delegate
}

func parseRedisURL(redisURL string) (string, string) {
	redisInfo, _ := url.Parse(redisURL)
	server := redisInfo.Host
	password := ""
	if redisInfo.User != nil {
		password, _ = redisInfo.User.Password()
	}
	return server, password
}

func createLock(c *gin.Context) {
	redisClient := redisPool.Get()
	expectedCommand := os.Getenv("LOCK_COMMAND")
	expectedToken := os.Getenv("LOCK_APP_TOKEN")
	receivedToken := c.PostForm("token")
	if strings.TrimSpace(expectedToken) != strings.TrimSpace(receivedToken) {
		c.AbortWithStatus(401)
		return
	}
	command := c.PostForm("command")
	if strings.TrimSpace(command) != expectedCommand {
		c.AbortWithStatus(400)
		return
	}
	lockKey := strings.ToLower(c.PostForm("text"))
	key := fmt.Sprintf("%s%s", lockPrefix, lockKey)
	username := c.PostForm("user_name")

	result, err := redisClient.Do("SETNX", key, fmt.Sprintf("%s--%s", username, time.Now().Format(time.RFC822Z)))
	if err != nil {
		handleRedisError(c, err)
	} else if result.(int64) == 0 {
		s, err := redisClient.Do("Get", key)
		if err != nil {
			handleRedisError(c, err)
		} else {
			lockDetails := strings.Split(string(s.([]byte)), "--")
			owner := lockDetails[0]
			ageMessage := getAgeMessage(lockDetails[1])
			if err != nil {
				handleRedisError(c, err)
			} else if owner == username {
				c.JSON(410, gin.H{
					"response_type": "in_channel",
					"text":          fmt.Sprintf("No worries, you already have the lock on %s (%s)", lockKey, ageMessage),
				})
			} else {
				c.JSON(410, gin.H{
					"response_type": "in_channel",
					"text":          fmt.Sprintf(":x: Sorry, lock currently held by: %s (%s)", owner, ageMessage),
				})
			}
		}
	} else {
		c.JSON(200, gin.H{
			"response_type": "in_channel",
			"text":          fmt.Sprintf("%s is now locked by %s", lockKey, username),
			"icon_emoji":    ":lock:",
		})
	}
}

func viewLock(c *gin.Context) {
	redisClient := redisPool.Get()
	expectedCommand := os.Getenv("VIEW_LOCK_COMMAND")

	expectedToken := os.Getenv("VIEW_LOCK_APP_TOKEN")
	receivedToken := c.PostForm("token")
	if strings.TrimSpace(expectedToken) != strings.TrimSpace(receivedToken) {
		c.AbortWithStatus(401)
		return
	}
	command := c.PostForm("command")
	if strings.TrimSpace(command) != expectedCommand {
		c.AbortWithStatus(400)
		return
	}
	lockKey := strings.ToLower(c.PostForm("text"))
	key := fmt.Sprintf("%s%s", lockPrefix, lockKey)
	value, err := redisClient.Do("GET", key)
	if value == nil {
		c.JSON(200, gin.H{
			"response_type": "in_channel",
			"text":          "That lock is not currently held",
		})

	} else {
		lockDetails := strings.Split(string(value.([]byte)), "--")
		owner := lockDetails[0]
		ageMessage := getAgeMessage(lockDetails[1])
		if err != nil {
			handleRedisError(c, err)
		}
		c.JSON(200, gin.H{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("%s is held by (%s) %s", lockKey, owner, ageMessage),
		})
	}
}

func unlock(c *gin.Context) {
	expectedCommand := os.Getenv("UNLOCK_COMMAND")
	expectedToken := os.Getenv("UNLOCK_APP_TOKEN")
	receivedToken := c.PostForm("token")
	if strings.TrimSpace(expectedToken) != strings.TrimSpace(receivedToken) {
		c.AbortWithStatus(401)
		return
	}
	command := c.PostForm("command")
	if strings.TrimSpace(command) != expectedCommand {
		c.AbortWithStatus(400)
		return
	}
	lockKey := strings.ToLower(c.PostForm("text"))
	text := fmt.Sprintf("%s%s", lockPrefix, lockKey)
	username := c.PostForm("user_name")

	result, err := redisPool.Get().Do("SETNX", fmt.Sprintf("unlock-attempt-%s", text), true)
	defer redisPool.Get().Do("DEL", fmt.Sprintf("unlock-attempt-%s", text), true)
	if err != nil {
		handleRedisError(c, err)
	} else {
		if result.(int64) == 0 {
			c.JSON(200, gin.H{
				"response_type": "in_channel",
				"text":          "Hmmm, someone else is trying to unlock this. How odd.",
			})
		} else {
			result, err = redisPool.Get().Do("GET", text)
			if err != nil {
				handleRedisError(c, err)
				return
			}
			owner := strings.Split(string(result.([]byte)), "--")[0]
			if owner != username {
				c.JSON(400, gin.H{
					"response_type": "in_channel",
					"text":          fmt.Sprintf(":rage4: Come on, you can't unlock %s's lock.", string(result.([]byte))),
				})
			} else {
				result, err = redisPool.Get().Do("DEL", text)
				if err != nil {
					handleRedisError(c, err)
				} else {
					if result.(int64) == 0 {
						c.JSON(200, gin.H{
							"response_type": "in_channel",
							"text":          fmt.Sprintf("That was never locked. What's your problem?"),
						})
					} else {

						c.JSON(410, gin.H{
							"response_type": "in_channel",
							"text":          fmt.Sprintf("%s is now available", lockKey),
						})
					}
				}

			}

		}
	}
}

func handleRedisError(c *gin.Context, err error) {
	fmt.Fprintln(os.Stderr, err)
	c.JSON(502, gin.H{
		"response_type": "in_channel",
		"text":          ":bomb: Something is wrong with the lock service",
	})
}

func getAgeMessage(created string) string {
	parsedCreationTime, err := time.Parse(time.RFC822Z, created)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to parse create date")
		return "Unknown lifetime"
	}
	age := time.Since(parsedCreationTime)
	total := int(age.Minutes())
	var days, hours, minutes int
	remainder := total
	if total > day {
		days = int(total / day)
		remainder = remainder % days
	}
	hours = int(remainder / 60)
	minutes = total % 60
	if days == 0 && hours == 0 && minutes == 0 {
		return "For less than a minute"
	}
	var daysString, hoursString, minutesString string
	if days > 0 {
		daysString = fmt.Sprintf("%d days ", days)
	} else {
		daysString = ""
	}
	if hours > 0 {
		hoursString = fmt.Sprintf("%d hours ", hours)
	} else {
		hoursString = ""
	}
	if minutes > 0 {
		minutesString = fmt.Sprintf("%d minutes", minutes)
	} else {
		minutesString = ""
	}
	return fmt.Sprint(daysString, hoursString, minutesString)
}

func listKeys(c *gin.Context) {
	redisClient := redisPool.Get()
	expectedCommand := os.Getenv("LIST_LOCK_COMMAND")
	expectedToken := os.Getenv("LIST_LOCK_APP_TOKEN")
	receivedToken := c.PostForm("token")
	if strings.TrimSpace(expectedToken) != strings.TrimSpace(receivedToken) {
		c.AbortWithStatus(401)
		return
	}
	command := c.PostForm("command")
	if strings.TrimSpace(command) != expectedCommand {
		c.AbortWithStatus(400)
		return
	}
	results, err := redisClient.Do("KEYS", fmt.Sprintf("%s*", lockPrefix))
	keys := []string{"Current locks: "}
	if err != nil {
		handleRedisError(c, err)
	} else {
		userName := c.PostForm("text")
		if strings.ToLower(userName) == "me" {
			userName = c.PostForm("user_name")
		}
		for _, result := range results.([]interface{}) {
			key := string(result.([]byte))
			lockKey := strings.Replace(key, lockPrefix, "", -1)
			value, err := redisClient.Do("GET", key)
			lockDetails := strings.Split(string(value.([]byte)), "--")
			plainOwner := lockDetails[0]
			owner := strings.ToLower(plainOwner)
			if err != nil {
				handleRedisError(c, err)
			}
			if strings.ToLower(userName) == "" || userName == owner {
				keys = append(keys, fmt.Sprintf("%s locked by %s (%s)", lockKey, plainOwner, getAgeMessage(lockDetails[1])))
			}
		}
		c.JSON(200, gin.H{
			"response_type": "emphemeral",
			"text":          strings.Join(keys, "\n"),
		})
	}
}

func main() {
	server, password := parseRedisURL(os.Getenv("REDIS_URL"))
	redisPool = newRedisPool(server, password)
	r := gin.Default()
	r.POST("/lock", createLock)
	r.POST("/unlock", unlock)
	r.POST("/listlocks", listKeys)
	r.POST("/viewlock", viewLock)
	r.Run(fmt.Sprintf(":%s", os.Getenv("PORT")))
}
