package producer

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProducer_Produce(t *testing.T) {
	p := NewProducer(30 * time.Second)
	for i := 1; i < 10001; i++ {
		p.Produce(i, GenerateSubId())
		if i%100 == 0 {
			time.Sleep(5 * time.Second)
		}
	}
}
func TestName(t *testing.T) {
	task := Task{
		ID:      11,
		Content: "content",
	}
	by, _ := json.Marshal(task)
	println(string(by))
}
