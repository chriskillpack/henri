package ollama

import "context"

func Init(srvAddr string) {

}

func DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}

func IsHealthy() bool {
	return true
}
