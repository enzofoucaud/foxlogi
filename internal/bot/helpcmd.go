package bot

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// HelpCommand implements /help. It renders the list of registered commands and
// their subcommands straight from the registry, so it never drifts from what
// the bot actually exposes.
type HelpCommand struct {
	registry *Registry
}

// NewHelpCommand creates the /help command backed by the registry it lists.
func NewHelpCommand(registry *Registry) *HelpCommand {
	return &HelpCommand{registry: registry}
}

// Definition describes the /help command.
func (c *HelpCommand) Definition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "help",
		Description: "List the bot's commands and what they do",
	}
}

// Handle replies (ephemerally) with every command and subcommand, sorted by name.
func (c *HelpCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	names := make([]string, 0, len(c.registry.commands))
	for name := range c.registry.commands {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		def := c.registry.commands[name].Definition()
		fmt.Fprintf(&b, "**/%s** — %s\n", def.Name, def.Description)
		for _, opt := range def.Options {
			if opt.Type == discordgo.ApplicationCommandOptionSubCommand {
				fmt.Fprintf(&b, "• `/%s %s` — %s\n", def.Name, opt.Name, opt.Description)
			}
		}
		b.WriteString("\n")
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🤖 Foxlogi commands",
		Description: strings.TrimSpace(b.String()),
		Color:       0x95A5A6,
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		slog.Error("respond help", "err", err)
	}
}
