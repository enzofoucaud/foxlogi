package bot

import (
	"context"
	"log"
	"time"

	"foxlogi/internal/guildconfig"

	"github.com/bwmarrin/discordgo"
)

const (
	// maxEmbedDescription leaves headroom under Discord's 4096-char embed limit.
	maxEmbedDescription = 3900
	// dbTimeout keeps repository calls well inside Discord's 3s interaction-ack window.
	dbTimeout = 3 * time.Second
)

// resolveChannel returns the configured channel for a guild via pick, falling
// back to origin when the setting is unset or the lookup fails.
func resolveChannel(ctx context.Context, settings guildconfig.Repository, guildID, origin string, pick func(guildconfig.Settings) string) string {
	set, err := settings.Get(ctx, guildID)
	if err != nil {
		log.Printf("guild settings lookup: %v", err)
		return origin
	}
	if ch := pick(set); ch != "" {
		return ch
	}
	return origin
}

// sendWithFallback posts msg to the primary channel; if that fails and the
// origin channel differs, it retries there. Both failures are logged.
func sendWithFallback(s *discordgo.Session, primary, origin, msg string) {
	if _, err := s.ChannelMessageSend(primary, msg); err == nil {
		return
	} else {
		log.Printf("send to channel %s failed: %v", primary, err)
	}
	if origin == "" || origin == primary {
		return
	}
	if _, err := s.ChannelMessageSend(origin, msg); err != nil {
		log.Printf("fallback send to channel %s failed: %v", origin, err)
	}
}

// optionMap indexes interaction options by name for easy lookup.
func optionMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, o := range opts {
		m[o.Name] = o
	}
	return m
}

// interactionUserName returns a display name (no ping) for the invoking user,
// preferring the guild nickname, then the username.
func interactionUserName(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		if i.Member.Nick != "" {
			return i.Member.Nick
		}
		if i.Member.User != nil {
			return i.Member.User.Username
		}
	}
	if i.User != nil {
		return i.User.Username
	}
	return "Someone"
}

// interactionUserID returns the invoking user's ID for both guild and DM interactions.
func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// replyEphemeral sends a private response visible only to the invoking user.
func replyEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("respond ephemeral: %v", err)
	}
}
