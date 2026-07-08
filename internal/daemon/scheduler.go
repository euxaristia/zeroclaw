package daemon

import (
	"context"
	"log"
	"time"

	"zeroclaw/internal/agent"
	"zeroclaw/internal/config"
)

const heartbeatPrompt = "Scheduled heartbeat. Read ~/HEARTBEAT.md and follow it exactly."

// startScheduler wires the daemon's tickers: the heartbeat plus any
// user-defined interval schedules from host config. Scheduled turns run
// through the same driver and conversation locks as user turns; a tick that
// finds its conversation busy is skipped, not queued.
func (s *server) startScheduler(ctx context.Context, cfg config.Config) {
	if d, ok := config.Interval(cfg.HeartbeatEvery); ok {
		go s.runEvery(ctx, d, "heartbeat", heartbeatPrompt)
		log.Printf("scheduler: heartbeat every %s", d)
	}
	for _, sched := range cfg.Schedules {
		d, ok := config.Interval(sched.Every)
		if !ok || sched.Prompt == "" {
			log.Printf("scheduler: skipping invalid schedule %q", sched.Name)
			continue
		}
		conv := sched.Conversation
		if conv == "" {
			conv = "sched-" + sched.Name
		}
		go s.runEvery(ctx, d, conv, sched.Prompt)
		log.Printf("scheduler: %q every %s -> conversation %q", sched.Name, d, conv)
	}
}

func (s *server) runEvery(ctx context.Context, every time.Duration, conversation, prompt string) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runScheduled(ctx, conversation, prompt)
		}
	}
}

// runScheduled fires one unattended turn. It is also the /beat handler's path.
func (s *server) runScheduled(ctx context.Context, conversation, prompt string) {
	lock := s.convLock(conversation)
	if !lock.TryLock() {
		log.Printf("scheduler: %q busy, tick skipped", conversation)
		return
	}
	defer lock.Unlock()

	opts := agent.TurnOptions{
		SessionID: s.sessions.Get(conversation),
		Prompt:    prompt,
		Autonomy:  "high",
	}
	res, err := s.driver.Turn(ctx, opts, nil)
	if res.SessionID != "" && opts.SessionID == "" {
		if serr := s.sessions.Set(conversation, res.SessionID); serr != nil {
			log.Printf("scheduler: persisting %q session: %v", conversation, serr)
		}
	}
	if err != nil {
		log.Printf("scheduler: %q turn failed: %v", conversation, err)
		return
	}
	log.Printf("scheduler: %q done status=%s final=%.120q", conversation, res.Status, res.Final)
}
