-- Bobber Marker draws solid magenta corner brackets around the player's own
-- fishing bobber while the player channels Fishing, for Visual Response Lab to
-- find on screen. The bot clicks the centre of the box and watches the box for
-- the splash, so the box is centred on the bobber and drawn in exact pixels.
--
-- Addons cannot ask where a world object is on screen, but the game's soft
-- targeting (the "softinteract" unit) can give the bobber a nameplate, and the
-- brackets hang on that nameplate. The same approach as ForeverLootSparkles and
-- NaowhForever's Glimmer, which draws softer brackets that change colour with
-- the water behind them.

local SOFT = "softinteract";
local BOBBER = 35591; -- game object ID of the fishing bobber, the same in every language
local GUID_OBJECT = "^GameObject%-%d+%-%d+%-%d+%-%d+%-(%d+)%-";

-- In screen pixels. Visual Response Lab's locator expects arms of at least 10
-- pixels and a box side between 30 and 400 pixels.
local BOX = 96;
local ARM = 20;
local THICKNESS = 3;
local DEFAULT_Y = -20; -- the bobber sits this far below its nameplate's centre
local COLOR = { 1, 0, 1 }; -- opaque magenta: exact over any background, and rare in the world

local POLL = 0.1; -- seconds between checks while fishing

-- Soft targeting must be on for the bobber to become the soft target. These
-- are set once, the first time you fish, and stay as you set them afterwards.
local NEEDED = {
	SoftTargetInteract = "3",            -- the game's "Enable Interact Key"
	SoftTargetIconGameObject = "1",      -- world objects can get a nameplate
	SoftTargetInteractOnlyInRange = "0", -- the bobber is often beyond interact range
	SoftTargetInteractRange = "60",      -- the game still caps it
};
-- The nameplate itself is only on while you fish, then goes back to your value.
local PLATE = "SoftTargetNameplateInteract";

local db;
local fishing = false;
local plateBefore; -- your SoftTargetNameplateInteract while the marker had it on
local anchor;      -- the nameplate the marker hangs on

local function Readable(value)
	return value ~= nil and not (issecretvalue and issecretvalue(value));
end

local function GetCVarValue(name)
	local ok, value = pcall(C_CVar.GetCVar, name);
	if ok and Readable(value) then
		return value;
	end
end

-- Writes only real changes: every write runs other addons' CVAR_UPDATE handlers.
local function SetCVarValue(name, value)
	local current = GetCVarValue(name);
	if current == nil or current == value or InCombatLockdown() then
		return;
	end
	pcall(C_CVar.SetCVar, name, value);
end

---------------------------------------------------------------------------
-- Marker
---------------------------------------------------------------------------

local marker = CreateFrame("Frame");
marker:Hide();
local bars = {};
for i = 1, 8 do
	local bar = marker:CreateTexture(nil, "OVERLAY");
	bar:SetColorTexture(COLOR[1], COLOR[2], COLOR[3], 1);
	if bar.SetSnapToPixelGrid then
		bar:SetSnapToPixelGrid(true);
		bar:SetTexelSnappingBias(0);
	end
	bars[i] = bar;
end

-- UI units per screen pixel for a frame, whatever the UI scale.
local function PixelSize(frame)
	local _, height = GetPhysicalScreenSize();
	return 768 / height / frame:GetEffectiveScale();
end

-- Sizes the box and its brackets in screen pixels. Each corner gets a
-- horizontal and a vertical bar that meet in the corner.
local function Layout()
	local px = PixelSize(marker);
	marker:SetSize(BOX * px, BOX * px);
	local i = 1;
	for _, corner in ipairs({ "TOPLEFT", "TOPRIGHT", "BOTTOMLEFT", "BOTTOMRIGHT" }) do
		for _, size in ipairs({ { ARM, THICKNESS }, { THICKNESS, ARM } }) do
			local bar = bars[i];
			bar:ClearAllPoints();
			bar:SetPoint(corner, marker, corner, 0, 0);
			bar:SetSize(size[1] * px, size[2] * px);
			i = i + 1;
		end
	end
end

local function Release()
	if not anchor then
		return;
	end
	marker:Hide();
	marker:ClearAllPoints();
	marker:SetParent(nil);
	anchor = nil;
end

local function Attach(plate)
	marker:SetParent(plate);
	marker:SetIgnoreParentAlpha(true);
	local ok, level = pcall(plate.GetFrameLevel, plate);
	if ok and Readable(level) then
		marker:SetFrameLevel(level + 10); -- above the nameplate's own parts
	end
	Layout();
	marker:ClearAllPoints();
	marker:SetPoint("CENTER", plate, "CENTER", 0, db.y * PixelSize(marker));
	marker:Show();
	anchor = plate;
end

local function IsBobber(unit)
	local ok, guid = pcall(UnitGUID, unit);
	if not ok or not Readable(guid) or type(guid) ~= "string" then
		return false;
	end
	return tonumber(guid:match(GUID_OBJECT)) == BOBBER;
end

-- Shows the marker while you fish and the soft target is your bobber with a nameplate.
local function Evaluate()
	if not fishing or not IsBobber(SOFT) then
		return Release();
	end
	local ok, plate = pcall(C_NamePlate.GetNamePlateForUnit, SOFT);
	if not ok or not Readable(plate) or not plate then
		return Release();
	end
	if plate ~= anchor then
		Release();
		Attach(plate);
	end
end

---------------------------------------------------------------------------
-- Fishing
---------------------------------------------------------------------------

local poller = CreateFrame("Frame");
poller:Hide();
local sinceCheck = 0;
poller:SetScript("OnUpdate", function(_, elapsed)
	sinceCheck = sinceCheck + elapsed;
	if sinceCheck >= POLL then
		sinceCheck = 0;
		Evaluate();
	end
end);

local function StartFishing()
	fishing = true;
	if not db.prepared and not InCombatLockdown() then
		for name, value in pairs(NEEDED) do
			SetCVarValue(name, value);
		end
		db.prepared = true;
	end
	if plateBefore == nil then
		plateBefore = GetCVarValue(PLATE);
	end
	SetCVarValue(PLATE, "1");
	poller:Show();
	Evaluate();
end

local function StopFishing()
	fishing = false;
	poller:Hide();
	Release();
	if plateBefore ~= nil then
		SetCVarValue(PLATE, plateBefore);
		plateBefore = nil;
	end
end

-- Fishing is a channel, so any channel of yours starts watching; the bobber
-- check decides what is marked. The language of the spell name never matters.
local function IsChanneling()
	local ok, name = pcall(UnitChannelInfo, "player");
	return ok and Readable(name) and name ~= nil;
end

---------------------------------------------------------------------------
-- Events and the slash command
---------------------------------------------------------------------------

local handlers = {};

-- Called for this addon only; see the event frame below.
function handlers.ADDON_LOADED()
	BobberMarkerDB = BobberMarkerDB or {};
	db = BobberMarkerDB;
	db.y = db.y or DEFAULT_Y;
end

function handlers.PLAYER_ENTERING_WORLD()
	if IsChanneling() then
		StartFishing();
	else
		StopFishing();
	end
end

function handlers.UNIT_SPELLCAST_CHANNEL_START(unit)
	if unit == "player" then
		StartFishing();
	end
end

function handlers.UNIT_SPELLCAST_CHANNEL_STOP(unit)
	if unit == "player" then
		StopFishing();
	end
end

function handlers.NAME_PLATE_UNIT_REMOVED(unit)
	local ok, plate = pcall(C_NamePlate.GetNamePlateForUnit, unit);
	if ok and plate and plate == anchor then
		Release();
	end
end

handlers.NAME_PLATE_UNIT_ADDED = Evaluate;
handlers.PLAYER_SOFT_INTERACT_CHANGED = Evaluate;

local addonName = ...;
local events = CreateFrame("Frame");
events:SetScript("OnEvent", function(_, event, arg1)
	if event == "ADDON_LOADED" then
		if arg1 == addonName then
			handlers.ADDON_LOADED();
		end
		return;
	end
	if db then
		handlers[event](arg1);
	end
end);
for event in pairs(handlers) do
	pcall(events.RegisterEvent, events, event);
end

SLASH_BOBBERMARKER1 = "/bobbermarker";
SlashCmdList.BOBBERMARKER = function(input)
	local value = tonumber((input or ""):match("^%s*y%s+(%-?%d+)%s*$"));
	if value then
		db.y = value;
		if anchor then
			local plate = anchor;
			Release();
			Attach(plate);
		end
	end
	print(("|cffff00ffBobber Marker:|r the box is %d pixels from the nameplate's centre. Move it with /bobbermarker y <pixels>, negative is down (default %d)."):format(db.y, DEFAULT_Y));
end
