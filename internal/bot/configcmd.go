package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"foxlogi/internal/events"
	"foxlogi/internal/guildconfig"

	"github.com/bwmarrin/discordgo"
)

// ConfigCommand implements the admin-only /config command that sets where craft
// and request messages are posted on a guild.
type ConfigCommand struct {
	settings guildconfig.Repository
	recorder events.Recorder
}

// NewConfigCommand creates the /config command backed by the settings repo.
func NewConfigCommand(settings guildconfig.Repository, recorder events.Recorder) *ConfigCommand {
	return &ConfigCommand{settings: settings, recorder: recorder}
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
	roleOpt := func(desc string) []*discordgo.ApplicationCommandOption {
		return []*discordgo.ApplicationCommandOption{
			{Name: "role", Description: desc, Type: discordgo.ApplicationCommandOptionRole, Required: true},
		}
	}
	return &discordgo.ApplicationCommand{
		Name:                     "config",
		Description:              "Configure the bot's channels and building-code access",
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
				Name:        "building-role-add",
				Description: "Allow a role to view building codes",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     roleOpt("Role allowed to view building codes"),
			},
			{
				Name:        "building-role-remove",
				Description: "Revoke a role's access to building codes",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     roleOpt("Role to revoke"),
			},
			{
				Name:        "show",
				Description: "Show the current configuration",
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
		c.setChannel(ctx, s, i, sub, c.settings.SetCraftChannel, "Craft", "config.craft_channel")
	case "request-channel":
		c.setChannel(ctx, s, i, sub, c.settings.SetRequestChannel, "Request", "config.request_channel")
	case "building-role-add":
		c.setBuildingRole(ctx, s, i, sub, c.settings.AddBuildingRole, "now", "config.building_role_add")
	case "building-role-remove":
		c.setBuildingRole(ctx, s, i, sub, c.settings.RemoveBuildingRole, "no longer", "config.building_role_remove")
	case "show":
		c.handleShow(ctx, s, i)
	}
}

func (c *ConfigCommand) setBuildingRole(
	ctx context.Context,
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
	apply func(context.Context, string, string) error,
	verb string,
	eventType string,
) {
	roleID := optionMap(sub.Options)["role"].RoleValue(nil, "").ID
	if err := apply(ctx, i.GuildID, roleID); err != nil {
		log.Printf("set building role: %v", err)
		replyEphemeral(s, i, "❌ Failed to update building access.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("✅ <@&%s> can %s view building codes.", roleID, verb))
	logEvent(c.recorder, i.GuildID, interactionUserID(i), eventType, map[string]string{"role_id": roleID})
}

func (c *ConfigCommand) setChannel(
	ctx context.Context,
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	sub *discordgo.ApplicationCommandInteractionDataOption,
	set func(context.Context, string, string) error,
	label string,
	eventType string,
) {
	channelID := optionMap(sub.Options)["channel"].ChannelValue(nil).ID
	if err := set(ctx, i.GuildID, channelID); err != nil {
		log.Printf("set %s channel: %v", label, err)
		replyEphemeral(s, i, "❌ Failed to save the configuration.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("✅ %s messages will now be posted in <#%s>.", label, channelID))
	logEvent(c.recorder, i.GuildID, interactionUserID(i), eventType, map[string]string{"channel_id": channelID})
}

func (c *ConfigCommand) handleShow(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	set, err := c.settings.Get(ctx, i.GuildID)
	if err != nil {
		log.Printf("get settings: %v", err)
		replyEphemeral(s, i, "❌ Failed to read the configuration.")
		return
	}
	roles, err := c.settings.ListBuildingRoles(ctx, i.GuildID)
	if err != nil {
		log.Printf("list building roles: %v", err)
		replyEphemeral(s, i, "❌ Failed to read the configuration.")
		return
	}
	buildingRoles := "_none (admins only)_"
	if len(roles) > 0 {
		mentions := make([]string, len(roles))
		for idx, r := range roles {
			mentions[idx] = fmt.Sprintf("<@&%s>", r)
		}
		buildingRoles = strings.Join(mentions, ", ")
	}

	replyEphemeral(s, i, fmt.Sprintf("📋 Configuration:\n• Craft channel: %s\n• Request channel: %s\n• Building-code roles: %s",
		channelMention(set.CraftChannelID), channelMention(set.RequestChannelID), buildingRoles))
}

// channelMention renders a channel link, or a placeholder when unset.
func channelMention(id string) string {
	if id == "" {
		return "_not set (uses the command channel)_"
	}
	return fmt.Sprintf("<#%s>", id)
}
