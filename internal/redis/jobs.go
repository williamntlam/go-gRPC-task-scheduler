package redis

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// JobPayload represents the payload pushed to Redis queues
// TODO: Step 2 - Define the JobPayload struct
// It should have three fields:
// - TaskID (string, JSON tag: "task_id")
// - Type (string, JSON tag: "type")
// - Priority (string, JSON tag: "priority")
type JobPayload struct {
	TaskID   string `json:"task_id"`
	Type     string `json:"type"`
	Priority string `json:"priority"`
}

// PushJob pushes a job to the appropriate Redis priority queue
// priority should be one of: "critical", "high", "default", "low"
//
// TODO: Step 3 - Implement PushJob function
// Function signature: func PushJob(ctx context.Context, client *redis.Client, jobID uuid.UUID, jobType, priority string) error
//
// Steps to implement:
// 1. Call getQueueName(priority) to get the queue name (e.g., "q:critical")
// 2. Validate that queueName is not empty (if empty, return an error)
// 3. Create a JobPayload struct with:
//    - TaskID: jobID.String()
//    - Type: jobType
//    - Priority: priority
// 4. Marshal the payload to JSON using json.Marshal()
// 5. Handle any marshaling errors
// 6. Use client.LPush(ctx, queueName, payloadJSON) to push to Redis
// 7. Check for errors from LPUSH and return them
// 8. Return nil on success
func PushJob(ctx context.Context, client *redis.Client, jobID uuid.UUID, jobType, priority string) error {
	// TODO: Step 3.1 - Get queue name using getQueueName helper
	// queueName := getQueueName(priority)

	// TODO: Step 3.2 - Validate queue name (return error if empty)

	// TODO: Step 3.3 - Create JobPayload struct

	// TODO: Step 3.4 - Marshal payload to JSON

	// TODO: Step 3.5 - Handle marshaling errors

	// TODO: Step 3.6 - Push to Redis using LPUSH
	// Hint: client.LPush(ctx, queueName, payloadJSON).Err()

	// TODO: Step 3.7 - Handle Redis errors and return

	return nil
}

// getQueueName converts a priority string to the corresponding queue name
// Returns empty string if priority is invalid
//
// TODO: Step 4 - Implement getQueueName function
// Function signature: func getQueueName(priority string) string
//
// Steps to implement:
// 1. Use a switch statement on priority
// 2. Map each priority to its queue name:
//    - "critical" -> "q:critical"
//    - "high" -> "q:high"
//    - "default" -> "q:default"
//    - "low" -> "q:low"
// 3. Return empty string for invalid priorities (default case)
func getQueueName(priority string) string {
	// TODO: Step 4.1 - Implement switch statement
	// switch priority {
	// case "critical":
	//     return "q:critical"
	// ... (add other cases)
	// default:
	//     return ""
	// }

	return ""
}
