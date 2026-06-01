package bot

import (
	"context"
	"fmt"
	"log"

	"foxlogi/internal/guildconfig"

	"github.com/bwmarrin/discordgo"
)

// ConfigCommand implements the admin-only /config command that sets where craft
// and request messages are posted on a guild.
type ConfigCommand struct {
	settings guildconfig.Repository
}

// NewConfigCommand creates the /config command backed by the settings repo.
func NewConfigCommand(settings guildconfig.Repository) *ConfigCommand {
	return &ConfigCommand{settings: settings}
}

// Definition describes /config; it is gated to members with Manage Server and
// is not available in DMs.
func (c *ConfigCommand) Definition() *discordgo.ApplicationCommand {
	manageServer := int64(discordgo.PermissionManageGuild)
	dmPermission := false
	channelOpt := func(desc string) []*discordgo.ApplicationCommandOption {
		return []*discordgo.ApplicationCommandOption{
			{
				Name:         "channel",
				Description:  desc,
				Type:         discordgo.ApplicationCommandOptionChannel,
				Required:     true,
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText},
			},
		}
	}
	return &discordgo.ApplicationCommand{
		Name:                     "config",
		Description:              "Configure where the bot posts craft and request messages",
		DefaultMemberPermissions: &manageServer,
		DMPermission:             &dmPermission,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "craft-channel",
				Description: "Set the channel where craft messages are posted",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     channelOpt("Target channel for craft messages"),
			},
			{
				Name:        "request-channel",
				Description: "Set the channel where request messages are posted",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     channelOpt("Target channel for request messages"),
			},
			{
				Name:        "show",
				Description: "Show the current channel configuration",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
	}
}

// Handle dispatches to the matching subcommand.
func (c *ConfigCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}
	sub := data.Options[0]

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	switch sub.Name {
	case "craft-channel":
		c.setChannel(ctx, s, i, sub, c.settings.SetCraftChannel, "Craft")
	case "request-channel":
		c.setChannel(ctx, s, i, sub, c.settings.SetRequestChannel, "Request")
	case "show":
		c.handleShow(ctx, s, i)
	}
}

func (c *ConfigCommand) setChannel(
	ctx context.Context,
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
	set func(context.Context, string, string) error,
	label string,
) {
	channelID := optionMap(sub.Options)["channel"].ChannelValue(nil).ID
	if err := set(ctx, i.GuildID, channelID); err != nil {
		log.Printf("set %s channel: %v", label, err)
		replyEphemeral(s, i, "❌ Failed to save the configuration.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("✅ %s messages will now be posted in <#%s>.", label, channelID))
}

func (c *ConfigCommand) handleShow(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	set, err := c.settings.Get(ctx, i.GuildID)
	if err != nil {
		log.Printf("get settings: %v", err)
		replyEphemeral(s, i, "❌ Failed to read the configuration.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("📋 Channel configuration:\n• Craft: %s\n• Request: %s",
		channelMention(set.CraftChannelID), channelMention(set.RequestChannelID)))
}

// channelMention renders a channel link, or a placeholder when unset.
func channelMention(id string) string {
	if id == "" {
		return "_not set (uses the command channel)_"
	}
	return fmt.Sprintf("<#%s>", id)
}
