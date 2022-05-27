package zLog

import (
	"fmt"
	"testing"

	"go.uber.org/zap"
)

func Test(t *testing.T) {
	cfg := Config{
		Level:    InfoLevel,
		Console:  true,
		Filename: "./logs/test.log",
		MaxSize:  1,
	}
	err := InitLogger(&cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	for i := 0; i < 10; i++ {
		Debug("test Debug", zap.Int("key", i), zap.String("Debug", "Debuggggggggggggggggggggggggggggggggggggggggggg"))
		Info("test Info", zap.Int("key", i), zap.String("Info", "Infoooooooooooooooooooooooooooooooooooooooooooooooo"))
		Warn("test Warn", zap.Int("key", i), zap.String("Warn", "Warnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnn"))
		Error("test Error", zap.Int("key", i), zap.String("Error", "Errorrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"))
	}

}
