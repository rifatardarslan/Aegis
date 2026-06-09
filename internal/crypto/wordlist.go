package crypto

// BIP39Words contains the first 256 words of the standard BIP-39 English wordlist.
// This is sufficient for deriving a 3-word Short Authentication String (SAS)
// from 3 bytes of fingerprint XOR (values 0-255).
var BIP39Words = []string{
	"abandon", "ability", "able", "about", "above", "absent", "absorb", "abstract", "absurd", "abuse",
	"access", "accident", "account", "accuse", "achieve", "acid", "acoustic", "acquire", "across", "act",
	"action", "actor", "actress", "actual", "adapt", "add", "addict", "address", "adjust", "admit",
	"adult", "advance", "advice", "advise", "aerobic", "affair", "afford", "afraid", "again", "age",
	"agent", "agree", "ahead", "aim", "air", "airport", "aisle", "alarm", "album", "alcohol",
	"alert", "alien", "all", "alley", "allow", "almost", "alone", "alpha", "already", "also",
	"alter", "always", "amateur", "amazing", "among", "amount", "amused", "analyst", "anchor", "ancient",
	"anger", "angle", "angry", "animal", "ankle", "announce", "annual", "another", "answer", "antenna",
	"antique", "anxiety", "any", "apart", "apology", "appear", "apple", "approve", "april", "arch",
	"arctic", "area", "arena", "argue", "arm", "armed", "armor", "army", "around", "arrange",
	"arrest", "arrive", "arrow", "art", "artefact", "artist", "artwork", "ask", "aspect", "assault",
	"asset", "assist", "assume", "asthma", "athlete", "atom", "attack", "attend", "attitude", "attract",
	"auction", "audit", "august", "aunt", "author", "auto", "autumn", "average", "avocado", "avoid",
	"awake", "award", "aware", "away", "awesome", "awful", "awkward", "axis", "baby", "bachelor",
	"bacon", "badge", "bag", "balance", "balcony", "ball", "bamboo", "banana", "banner", "bar",
	"barely", "bargain", "barrel", "barrier", "base", "basic", "basket", "battle", "beauty", "because",
	"become", "beef", "before", "begin", "behave", "behind", "believe", "below", "belt", "bench",
	"benefit", "best", "betray", "better", "between", "beyond", "bicycle", "bid", "bike", "bind",
	"biology", "bird", "birth", "bitter", "black", "blade", "blame", "blanket", "blast", "bleak",
	"bless", "blind", "blood", "blossom", "blouse", "blue", "blur", "blush", "board", "boat",
	"body", "boil", "bomb", "bone", "bonus", "book", "boost", "border", "boring", "borrow",
	"boss", "bottom", "bounce", "box", "boy", "bracket", "brain", "brand", "brass", "brave",
	"bread", "breeze", "bright", "bring", "brisk", "broccoli", "broken", "bronze", "broom", "brother",
	"brown", "brush", "bubble", "buddy", "budget", "buffalo", "build", "bulb", "bulk", "bullet",
	"bundle", "bunker", "burden", "burger", "burst", "bus", "business", "busy", "butter", "buyer",
	"buzz", "cabbage", "cabin", "cable", "cactus", "cage",
}
