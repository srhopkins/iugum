package agentcore

import (
	"fmt"
	"os"
	"path/filepath"
)

func (r *Runtime) subscriptionVision(model string) bool {
	for _, allowed := range r.config.SubscriptionVisionModels {
		if allowed == model && model != "" {
			return true
		}
	}
	return false
}
func writeSubscriptionImages(dir string, messages []Message) ([]string, error) {
	if err := imageAggregate(messages); err != nil {
		return nil, err
	}
	files := []string{}
	for _, message := range messages {
		images, err := validatedImages(message.Images)
		if err != nil {
			return nil, err
		}
		for _, im := range images {
			path := filepath.Join(dir, fmt.Sprintf("attachment-%03d.png", len(files)+1))
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return nil, fmt.Errorf("agentcore: create private image attachment: %w", err)
			}
			_, err = file.Write(im.Data)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				return nil, fmt.Errorf("agentcore: write private image attachment failed")
			}
			files = append(files, path)
		}
	}
	return files, nil
}
