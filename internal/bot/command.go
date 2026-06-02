// Package bot wires Discord slash commands and notifications to the craft domain.
package bot

import (
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Command is a single slash command. Adding a new feature to the bot means
// implementing this interface and passing it to NewRegistry — nothing else in
// the bot layer needs to change.
type Command interface {
	// Definition returns the Discord application command schema to register.
	Definition() *discordgo.ApplicationCommand
	// Handle processes an incoming interaction for this command.
	Handle(s *discordgo.Session, i *discordgo.InteractionCreate)
}

// Autocompleter is an optional interface a Command can implement to serve
// option autocomplete suggestions.
type Autocompleter interface {
	Autocomplete(s *discordgo.Session, i *discordgo.InteractionCreate)
}

// ModalSubmitter is an optional interface a Command can implement to handle
// modal submissions. The modal's CustomID must be prefixed with the command
// name (e.g. "building:add") so the registry can route it.
type ModalSubmitter interface {
	ModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate)
}

// Registry holds the bot's commands and dispatches interactions to them.
type Registry struct {
	commands map[string]Command
}

// NewRegistry builds the registry from the given commands, keyed by name.
func NewRegistry(cmds ...Command) *Registry {
	r := &Registry{commands: make(map[string]Command, len(cmds))}
	for _, c := range cmds {
		r.commands[c.Definition().Name] = c
	}
	return r
}

// Add registers an additional command after construction (used for commands
// that need a reference to the registry itself, e.g. /help).
func (r *Registry) Add(cmd Command) {
	r.commands[cmd.Definition().Name] = cmd
}

// Register publishes every command's definition to Discord for the given guild.
// An empty guildID registers the commands globally.
func (r *Registry) Register(s *discordgo.Session, guildID string) error {
	for _, c := range r.commands {
		if _, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, c.Definition()); err != nil {
			return err
		}
	}
	return nil
}

// Dispatch routes a command interaction (or its autocomplete) to the handler.
func (r *Registry) Dispatch(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		data := i.ApplicationCommandData()
		cmd, ok := r.commands[data.Name]
		if !ok {
			slog.Warn("no handler for command", "command", data.Name)
			return
		}
		// Log command/subcommand/user/guild only — never option values (secrets).
		slog.Info("command",
			"command", data.Name, "subcommand", subcommandName(data.Options),
			"user", interactionUserID(i), "guild", i.GuildID)
		cmd.Handle(s, i)
	case discordgo.InteractionApplicationCommandAutocomplete:
		data := i.ApplicationCommandData()
		cmd, ok := r.commands[data.Name]
		if !ok {
			return
		}
		if ac, ok := cmd.(Autocompleter); ok {
			slog.Debug("autocomplete", "command", data.Name, "user", interactionUserID(i), "guild", i.GuildID)
			ac.Autocomplete(s, i)
		}
	case discordgo.InteractionModalSubmit:
		customID := i.ModalSubmitData().CustomID
		name, _, _ := strings.Cut(customID, ":")
		cmd, ok := r.commands[name]
		if !ok {
			return
		}
		if ms, ok := cmd.(ModalSubmitter); ok {
			slog.Info("modal submit", "custom_id", customID, "user", interactionUserID(i), "guild", i.GuildID)
			ms.ModalSubmit(s, i)
		}
	}
}

// subcommandName returns the invoked subcommand name, or "" if there isn't one.
func subcommandName(opts []*discordgo.ApplicationCommandInteractionDataOption) string {
	if len(opts) > 0 && opts[0].Type == discordgo.ApplicationCommandOptionSubCommand {
		return opts[0].Name
	}
	return ""
}
