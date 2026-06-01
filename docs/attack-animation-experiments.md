# Attack Animation Experiments

This document records the PixelLab v3 object-animation experiments run against the lich knight smoke-test object while trying to produce attack frames that clearly read as combat.

Object used:

- `d4746b54-3145-4642-997f-5d184a70ac1c`

## Baseline Problem

The first short combat attempt used a softened/enhanced prompt and only `4` generated frames. It mostly moved the sword slightly and did not read as an attack.

Observed issues:

- Too few frames for anticipation, strike, follow-through, and recovery.
- `--enhance-prompt` softened the attack into subtle sword lifting.
- Pure "attack" or "slash" wording was too vague.
- Without explicit pose beats, PixelLab produced idle-like motion.

## Strongest Attack Read

Best source animation:

- Display name: `attack_physical_16_v2`
- Group: `44b19b19-9d06-4c90-9028-6649631dd22e`
- Frames reported by PixelLab: `17` because v3 stores the input/reference frame plus generated frames.

Best selected local output:

- `tmp/lichknight-attack-selected-9/`

Why it worked:

- `16` requested frames gave enough room for wind-up, active strike, follow-through, and recovery.
- The prompt described concrete animation beats instead of just naming the action.
- The prompt emphasized silhouette and sword-position change.
- Downselecting to `9` frames produced a tighter in-game cycle.

Tradeoff:

- PixelLab still added cyan sword-trail VFX, even with explicit negative wording.
- A post-processing pass can reduce large cyan components, but cannot fully remove them without risking sword and eye details.

Prompt shape that worked best for attack readability:

```text
side-view undead lich knight performs a heavy physical longsword melee attack for a game sprite. Starts from idle, raises the plain steel sword high behind the shoulder in a big wind-up, torso twists back, knees bend. Then he steps or lunges only slightly while mostly planted and chops downward-forward with the sword, creating a clear combat silhouette. The blade changes position dramatically from high wind-up to low extended follow-through, then returns to idle. Emphasize body pose, sword position, anticipation, strike frame, follow-through, and recovery. Keep same character, same armor, same sword, same right-facing side-view pixel art, transparent background. No magic effects, no glowing slash trail, no energy wave, no projectile, no blue arc, no impact burst, no walking cycle, no idle breathing, no jumping, no spinning, no camera movement, no extra weapons, no new objects, no background.
```

## Cleanest No-FX Result

Best clean source animation:

- Display name: `attack_pose_only_chop_12_v8`
- Group: `bc0fcfc6-e562-4065-bccd-365c520c41cd`
- Frames reported by PixelLab: `13` because v3 stores the input/reference frame plus generated frames.

Best selected local output:

- `tmp/lichknight-attack-clean-selected-9/`

Why it worked:

- Avoided words that tended to trigger VFX: `dramatic`, `impact`, `slash`, `arc`, `energy`, and `burst`.
- Used "pose only", "silhouette", and "solid dark grey blade" language.
- Focused on a controlled overhead chop rather than a high-impact attack.

Tradeoff:

- Much cleaner visually.
- Less aggressive and less obviously damaging than the VFX-heavy physical attack.

Prompt shape that worked best for no-FX consistency:

```text
side-view right-facing undead lich knight performs a physical overhead chop with strong anticipation and no effects. Start idle. Raise the plain dark steel sword high above and behind the helmet. Hold a clear wind-up pose. Bring the sword down to a low forward guard. Recover to idle. The attack reads from silhouette and pose only. The sword remains a solid dark grey blade with a small white edge, never cyan. No slash effect, no motion trail, no motion blur, no colored weapon, no magic, no energy, no sparks, no impact, no projectile, no crescent shape, no extra object, no walking, no jumping, no camera movement, transparent background.
```

## Other Experiment Results

### `attack_overhead_16_v1`

Result:

- Clearly read as an attack.
- Overproduced cyan slash VFX and large energy shapes.

Lesson:

- "Huge readable", "wide bright arc", and similar wording improves action readability but strongly encourages magical slash effects.

### `attack_interpolation_16_v3`

Result:

- Used a generated lunge/impact object state as the end frame.
- Composition was cleaner than the large VFX attempt.
- Drifted into green glow/power-up frames and lost the decisive slash.

Lesson:

- Interpolation is not automatically better for attacks. It can preserve a target pose but may weaken the action unless both start and end keyframes are very strong and style-consistent.

### `attack_restrained_slash_8_v4`

Result:

- Mostly avoided large VFX.
- Too subtle; read as a cautious poke rather than a combat attack.

Lesson:

- Reducing to `8` frames and using restrained language can reduce VFX, but it also weakens the attack read.

### `attack_thrust_12_v5`

Result:

- Good lunge/readability.
- Still added cyan hit effects near the end.

Lesson:

- Thrusts reduce slash arcs but do not fully prevent PixelLab from adding impact particles.

### `attack_oldschool_no_fx_16_v6`

Result:

- Good wind-up and pose progression.
- Still turned the sword cyan during strike frames.

Lesson:

- "Old-school" and "no special effects" are helpful but insufficient. The model still associates high-action sword motion with colored trails.

### `attack_training_chop_8_v7`

Result:

- Avoided blue VFX.
- Added yellow spark-like impact marks despite "no impact" language.

Lesson:

- Explicitly banning blue trails can shift artifacts into other VFX types, such as yellow sparks.

## Working Rules

- Avoid `--enhance-prompt` for precise combat animations unless the enhanced prompt is inspected before generation.
- Use `16` frames when the priority is attack readability, then downselect to `9` production frames.
- Use `12` or `8` frames when the priority is restrained/no-FX output, accepting weaker action.
- Include explicit beats: idle, anticipation, weapon raised, active strike, follow-through, recovery.
- Use silhouette language: body pose, arm pose, sword angle, blade position, readable combat silhouette.
- Avoid VFX-triggering terms when a clean physical attack is desired: `dramatic`, `impact`, `slash arc`, `energy`, `burst`, `wave`, and `bright`.
- Negative prompts reduce but do not guarantee removal of trails, sparks, or colored weapon artifacts.
- Manual downselection matters. The generated set often contains useful frames mixed with weak or over-effected frames.

## Recommended Production Choices

Use one of two variants depending on game feel:

- Punchier combat readability: `tmp/lichknight-attack-selected-9/`
- Cleaner low-fantasy/no-FX consistency: `tmp/lichknight-attack-clean-selected-9/`

Do not commit generated `tmp/` frames unless the repo intentionally starts tracking generated experiment artifacts. They are local review outputs; production assets should be copied into the target game repo under its normal asset path.
