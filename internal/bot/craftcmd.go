package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"foxlogi/internal/craft"

	"github.com/bwmarrin/discordgo"
)

const (
	// maxCraftDuration caps how far in the future a craft may be scheduled.
	maxCraftDuration = 30 * 24 * time.Hour
	// maxEmbedDescription leaves headroom under Discord's 4096-char embed limit.
	maxEmbedDescription = 3900
	// dbTimeout keeps repository calls well inside Discord's 3s interaction-ack window.
	dbTimeout = 3 * time.Second
)

// CraftCommand implements the /craft slash command (add and list subcommands).
type CraftCommand struct {
	repo          craft.Repository
	soonThreshold time.Duration
}

// NewCraftCommand creates the /craft command backed by repo.
func NewCraftCommand(repo craft.Repository, soonThreshold time.Duration) *CraftCommand {
	return &CraftCommand{repo: repo, soonThreshold: soonThreshold}
}

// Definition describes the /craft command and its subcommands.
func (c *CraftCommand) Definition() *discordgo.ApplicationCommand {
	minQty := 1.0
	dmPermission := false
	return &discordgo.ApplicationCommand{
		Name:         "craft",
		Description:  "Track Foxhole crafts and get notified when they are ready",
		DMPermission: &dmPermission,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "add",
				Description: "Register a craft with an estimated duration",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "item",
						Description: "Name of the item being crafted",
						Type:        discordgo.ApplicationCommandOptionString,
						Required:    true,
						MaxLength:   100,
					},
					{
						Name:        "duration",
						Description: "Time until ready, e.g. 30m or 1h30m",
						Type:        discordgo.ApplicationCommandOptionString,
						Required:    true,
					},
					{
						Name:        "quantity",
						Description: "How many (default 1)",
						Type:        discordgo.ApplicationCommandOptionInteger,
						MinValue:    &minQty,
						MaxValue:    100000,
					},
				},
			},
			{
				Name:        "list",
				Description: "List active crafts and their remaining time",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
	}
}

// Handle dispatches to the matching subcommand.
func (c *CraftCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}
	sub := data.Options[0]
	switch sub.Name {
	case "add":
		c.handleAdd(s, i, sub)
	case "list":
		c.handleList(s, i)
	}
}

func (c *CraftCommand) handleAdd(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := interactionUserID(i)
	if userID == "" {
		replyEphemeral(s, i, "⚠️ Could not identify you — please try again.")
		return
	}

	opts := optionMap(sub.Options)

	item := strings.TrimSpace(opts["item"].StringValue())
	if item == "" {
		replyEphemeral(s, i, "⚠️ Item name cannot be empty.")
		return
	}

	durStr := strings.TrimSpace(opts["duration"].StringValue())
	dur, err := time.ParseDuration(durStr)
	if err != nil || dur <= 0 {
		replyEphemeral(s, i, fmt.Sprintf("⚠️ Invalid duration %q. Use a format like `30m`, `2h`, or `1h30m`.", durStr))
		return
	}
	if dur > maxCraftDuration {
		replyEphemeral(s, i, "⚠️ Duration is too long (max 30d).")
		return
	}

	quantity := 1
	if q, ok := opts["quantity"]; ok {
		quantity = int(q.IntValue())
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	completion := time.Now().Add(dur)
	stored, err := c.repo.Add(ctx, craft.Craft{
		GuildID:    i.GuildID,
		ChannelID:  i.ChannelID,
		UserID:     userID,
		Item:       item,
		Quantity:   quantity,
		Completion: completion,
	})
	if err != nil {
		log.Printf("add craft: %v", err)
		replyEphemeral(s, i, "❌ Failed to save the craft, please try again.")
		return
	}

	replyEphemeral(s, i, fmt.Sprintf("✅ Tracking **%d× %s** (#%d) — ready <t:%d:R> (<t:%d:t>).",
		stored.Quantity, stored.Item, stored.ID, completion.Unix(), completion.Unix()))
}

func (c *CraftCommand) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	crafts, err := c.repo.ListByGuild(ctx, i.GuildID)
	if err != nil {
		log.Printf("list crafts: %v", err)
		replyEphemeral(s, i, "❌ Failed to fetch crafts.")
		return
	}
	if len(crafts) == 0 {
		replyEphemeral(s, i, "📭 No active crafts. Add one with `/craft add`.")
		return
	}

	now := time.Now()
	var b strings.Builder
	remaining := 0
	for idx, cr := range crafts {
		marker := "🕒"
		if cr.Remaining(now) <= c.soonThreshold {
			marker = "⏳ soon"
		}
		line := fmt.Sprintf("%s **%d× %s** (#%d) — ready <t:%d:R>\n",
			marker, cr.Quantity, cr.Item, cr.ID, cr.Completion.Unix())
		if b.Len()+len(line) > maxEmbedDescription {
			remaining = len(crafts) - idx
			break
		}
		b.WriteString(line)
	}
	if remaining > 0 {
		fmt.Fprintf(&b, "…and %d more", remaining)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🔨 Active crafts",
		Description: b.String(),
		Color:       0xE67E22,
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	}); err != nil {
		log.Printf("respond list: %v", err)
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
