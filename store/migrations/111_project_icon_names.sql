UPDATE project SET icon = '' WHERE icon NOT IN (
	'rocket', 'bolt', 'flame', 'wave', 'leaf', 'globe', 'star', 'bulb', 'flask', 'wrench',
	'box', 'puzzle', 'target', 'lock', 'chart', 'note', 'palette', 'gamepad', 'bot', 'brain',
	'cart', 'chat', 'phone', 'cloud', 'home', 'compass', 'code', 'book'
);
