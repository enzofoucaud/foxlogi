package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"foxlogi/internal/building"
	"foxlogi/internal/events"
	"foxlogi/internal/guildconfig"

	"github.com/bwmarrin/discordgo"
)

// buildingModalID is the CustomID of the password modal; its prefix ("building")
// routes the submit back to this command via the registry.
const buildingModalID = "building:add"

type buildingDraft struct {
	label, hexagon, town, btype, role string
}

// BuildingCommand implements /building (add, list, show, remove). Viewing is
// gated by the guild's configured building roles; passwords are only ever shown
// in ephemeral replies.
type BuildingCommand struct {
	repo     building.Repository
	settings guildconfig.Repository
	recorder events.Recorder

	mu      sync.Mutex
	pending map[string]buildingDraft // userID -> draft awaiting the password modal
}

// NewBuildingCommand creates the /building command.
func NewBuildingCommand(repo building.Repository, settings guildconfig.Repository, recorder events.Recorder) *BuildingCommand {
	return &BuildingCommand{repo: repo, settings: settings, recorder: recorder, pending: make(map[string]buildingDraft)}
}

// Definition describes /building and its subcommands.
func (c *BuildingCommand) Definition() *discordgo.ApplicationCommand {
	dm := false
	field := func(name, desc string) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{
			Name: name, Description: desc, Type: discordgo.ApplicationCommandOptionString, Required: true, MaxLength: 100,
		}
	}
	pick := func() []*discordgo.ApplicationCommandOption {
		return []*discordgo.ApplicationCommandOption{
			{Name: "building", Description: "Building", Type: discordgo.ApplicationCommandOptionInteger, Required: true, Autocomplete: true},
		}
	}
	return &discordgo.ApplicationCommand{
		Name:         "building",
		Description:  "Store and look up building access codes (role-restricted)",
		DMPermission: &dm,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "add",
				Description: "Add a building, then enter its code in a popup",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					field("label", "Short name, e.g. LIV GREAT"),
					field("hexagon", "Hex/region, e.g. Great March"),
					field("town", "Town, e.g. Sitaria"),
					field("type", "Type, e.g. Depot"),
					field("role", "Function, e.g. Faci/Logi HUB"),
				},
			},
			{Name: "list", Description: "List buildings (without their codes)", Type: discordgo.ApplicationCommandOptionSubCommand},
			{Name: "show", Description: "Reveal a building's access code (private)", Type: discordgo.ApplicationCommandOptionSubCommand, Options: pick()},
			{Name: "remove", Description: "Remove a building", Type: discordgo.ApplicationCommandOptionSubCommand, Options: pick()},
		},
	}
}

// authorize reports whether the member may use building commands on this guild.
func (c *BuildingCommand) authorize(ctx context.Context, i *discordgo.InteractionCreate) bool {
	roles, err := c.settings.ListBuildingRoles(ctx, i.GuildID)
	if err != nil {
		log.Printf("list building roles: %v", err)
		return false
	}
	return authorizedForBuildings(i, roles)
}

// Handle gates every subcommand, then dispatches.
func (c *BuildingCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}
	sub := data.Options[0]

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if !c.authorize(ctx, i) {
		replyEphemeral(s, i, "⛔ You don't have access to building codes.")
		return
	}

	switch sub.Name {
	case "add":
		c.handleAdd(s, i, sub)
	case "list":
		c.handleList(ctx, s, i)
	case "show":
		c.handleShow(ctx, s, i, sub)
	case "remove":
		c.handleRemove(ctx, s, i, sub)
	}
}

func (c *BuildingCommand) handleAdd(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := interactionUserID(i)
	if userID == "" {
		replyEphemeral(s, i, "⚠️ Could not identify you — please try again.")
		return
	}
	opts := optionMap(sub.Options)
	c.mu.Lock()
	c.pending[userID] = buildingDraft{
		label:   strings.TrimSpace(opts["label"].StringValue()),
		hexagon: strings.TrimSpace(opts["hexagon"].StringValue()),
		town:    strings.TrimSpace(opts["town"].StringValue()),
		btype:   strings.TrimSpace(opts["type"].StringValue()),
		role:    strings.TrimSpace(opts["role"].StringValue()),
	}
	c.mu.Unlock()

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: buildingModalID,
			Title:    "Building access code",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:  "password",
						Label:     "Password / access code",
						Style:     discordgo.TextInputShort,
						Required:  true,
						MaxLength: 100,
					},
				}},
			},
		},
	}); err != nil {
		log.Printf("open building modal: %v", err)
	}
}

// ModalSubmit finalises a building once the password modal is submitted.
func (c *BuildingCommand) ModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	if data.CustomID != buildingModalID {
		return
	}
	userID := interactionUserID(i)

	c.mu.Lock()
	draft, ok := c.pending[userID]
	delete(c.pending, userID)
	c.mu.Unlock()
	if !ok {
		replyEphemeral(s, i, "⚠️ Your draft expired (the bot may have restarted). Please re-run `/building add`.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	// Re-check access at commit time: the role could have been revoked between
	// opening the modal and submitting it.
	if !c.authorize(ctx, i) {
		replyEphemeral(s, i, "⛔ You don't have access to building codes.")
		return
	}

	password := strings.TrimSpace(modalTextValue(data, "password"))
	if password == "" {
		replyEphemeral(s, i, "⚠️ The password cannot be empty.")
		return
	}

	b, err := c.repo.AddBuilding(ctx, building.Building{
		GuildID:  i.GuildID,
		Label:    draft.label,
		Hexagon:  draft.hexagon,
		Town:     draft.town,
		Type:     draft.btype,
		Role:     draft.role,
		Password: password,
	})
	if err != nil {
		log.Printf("add building: %v", err)
		replyEphemeral(s, i, "❌ Failed to save the building.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("✅ Saved **%s** (#%d) — %s / %s. Reveal its code with `/building show`.",
		b.Label, b.ID, b.Hexagon, b.Town))

	// Never put the password in an event payload — list the safe fields explicitly.
	logEvent(c.recorder, i.GuildID, userID, "building.add", map[string]string{
		"label": b.Label, "hexagon": b.Hexagon, "town": b.Town,
	})
}

func (c *BuildingCommand) handleList(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	builds, err := c.repo.ListBuildingsByGuild(ctx, i.GuildID)
	if err != nil {
		log.Printf("list buildings: %v", err)
		replyEphemeral(s, i, "❌ Failed to fetch buildings.")
		return
	}
	if len(builds) == 0 {
		replyEphemeral(s, i, "📭 No buildings registered. Add one with `/building add`.")
		return
	}

	var b strings.Builder
	truncated := 0
	for idx, bd := range builds {
		line := fmt.Sprintf("**%s** (#%d) — %s / %s · %s · %s\n", bd.Label, bd.ID, bd.Hexagon, bd.Town, bd.Type, bd.Role)
		if b.Len()+len(line) > maxEmbedDescription {
			truncated = len(builds) - idx
			break
		}
		b.WriteString(line)
	}
	if truncated > 0 {
		fmt.Fprintf(&b, "…and %d more", truncated)
	}

	embed := &discordgo.MessageEmbed{Title: "🏚️ Buildings", Description: b.String(), Color: 0x9B59B6}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("respond building list: %v", err)
	}
}

func (c *BuildingCommand) handleShow(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	id := optionMap(sub.Options)["building"].IntValue()
	b, found, err := c.repo.GetBuilding(ctx, i.GuildID, id)
	if err != nil {
		log.Printf("get building: %v", err)
		replyEphemeral(s, i, "❌ Failed to load the building.")
		return
	}
	if !found {
		replyEphemeral(s, i, "⚠️ Building not found.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("🔐 **%s** — %s / %s (%s · %s)\nCode: **%s**",
		b.Label, b.Hexagon, b.Town, b.Type, b.Role, b.Password))

	// Audit: who viewed a code. Never log the password.
	logEvent(c.recorder, i.GuildID, interactionUserID(i), "building.show", map[string]string{
		"building_id": strconv.FormatInt(b.ID, 10), "label": b.Label,
	})
}

func (c *BuildingCommand) handleRemove(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	id := optionMap(sub.Options)["building"].IntValue()
	b, found, err := c.repo.GetBuilding(ctx, i.GuildID, id)
	if err != nil {
		log.Printf("get building: %v", err)
		replyEphemeral(s, i, "❌ Failed to load the building.")
		return
	}
	if !found {
		replyEphemeral(s, i, "⚠️ Building not found.")
		return
	}
	if err := c.repo.DeleteBuilding(ctx, i.GuildID, b.ID); err != nil {
		log.Printf("delete building: %v", err)
		replyEphemeral(s, i, "❌ Failed to remove the building.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("🗑️ Removed **%s** (#%d).", b.Label, b.ID))

	logEvent(c.recorder, i.GuildID, interactionUserID(i), "building.remove", map[string]string{
		"building_id": strconv.FormatInt(b.ID, 10), "label": b.Label,
	})
}

// Autocomplete suggests the guild's buildings for show/remove, but only to
// authorised members.
func (c *BuildingCommand) Autocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if !c.authorize(ctx, i) {
		respondAutocomplete(s, i, nil)
		return
	}
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		respondAutocomplete(s, i, nil)
		return
	}
	focused := focusedOption(data.Options[0].Options)
	if focused == nil || focused.Name != "building" {
		respondAutocomplete(s, i, nil)
		return
	}

	builds, err := c.repo.ListBuildingsByGuild(ctx, i.GuildID)
	if err != nil {
		respondAutocomplete(s, i, nil)
		return
	}
	prefix := strings.ToLower(optionString(focused))
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, b := range builds {
		if prefix != "" && !strings.Contains(strings.ToLower(b.Label), prefix) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  clampChoiceName(fmt.Sprintf("%s — %s / %s", b.Label, b.Hexagon, b.Town)),
			Value: b.ID,
		})
		if len(choices) == 25 {
			break
		}
	}
	respondAutocomplete(s, i, choices)
}
