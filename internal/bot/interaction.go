package bot

import (
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	// maxEmbedDescription leaves headroom under Discord's 4096-char embed limit.
	maxEmbedDescription = 3900
	// dbTimeout keeps repository calls well inside Discord's 3s interaction-ack window.
	dbTimeout = 3 * time.Second
)

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
