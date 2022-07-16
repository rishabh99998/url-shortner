package routes

import (
	"os"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rishabh-lt/go-url-shortner/database"
	"github.com/rishabh-lt/go-url-shortner/helpers"
)


type request struct{
	URL			string 	`json:"url"`
	CustomShort	string 	`json:"short"`
	Expiry		time.Duration 	`json:"expiry"`	
}

type response struct{
	URL             string        `json:"url"`
	CustomShort     string        `json:"short"`
	Expiry          time.Duration `json:"expiry"`
	XRateRemaining  int           `json:"rate_limit"`
	XRateLimitReset time.Duration `json:"rate_limit_reset"`
}

func ShortenURL(c *fiber.Ctx) error{
	body := request{}

	if err := c.BodyParser(&body); err != nil{
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// rate limiting 

	r1 := database.CreateClient(0)
	r1.Close()

	val,err := r1.Get(database.Ctx, c.IP()).Result()

	if err == redis.Nil{
		_ = r1.Set(database.Ctx,c.IP(),os.Getenv("API_QUOTA"), 30*60*time.Second)
	}else{
		valInt,_ := strconv.Atoi(val)
		if valInt <= 0{
			limit,_ := r1.TTL(database.Ctx, c.IP()).Result()
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":            "Rate limit exceeded!",
				"rate_limit_reset": limit / time.Nanosecond / time.Minute,
			})
		}
	}

	// check if input is a valid url

	if !govalidator.IsURL(body.URL){
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid URL!",
		})
	}

	// check for domin error => if user tries to hack our system by entering the same url as our system url
	if !helpers.RemoveDomainError(body.URL){
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "You can't access it! :p",
		})
	}

	// enfore https,SSL
	body.URL = helpers.EnforceHTTP(body.URL)

	// custom URL by user functionality
	var id string

	if body.CustomShort == ""{
		id = uuid.New().String()
	}else{
		id = body.CustomShort
	}

	r := database.CreateClient(1)

	val,_ = r.Get(database.Ctx, id).Result()

	if val!=""{
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Your custom shorl URL is already in use!",
		})
	}

	if body.Expiry == 0{
		body.Expiry = 24
	}

	err = r.Set(database.Ctx, id, body.URL, body.Expiry*3600*time.Second).Err()

	if err!= nil{
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server!",
		})
	}

	resp := response{
		URL:             body.URL,
		CustomShort:     "",
		Expiry:          body.Expiry,
		XRateRemaining:  10,
		XRateLimitReset: 30,
	}

	r1.Decr(database.Ctx, c.IP())

	val,_ =  r.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)
	ttl,_ := r.TTL(database.Ctx, c.IP()).Result()
	resp.XRateLimitReset = ttl / time.Nanosecond / time.Minute

	resp.CustomShort = os.Getenv("DOMAIN") + "/" + id

	return c.Status(fiber.StatusOK).JSON(resp)

}