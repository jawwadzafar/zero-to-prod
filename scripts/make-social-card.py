# Generates static/img/social-card.png (1200x630), the preview image shown when
# a link to the site is shared. Re-run after changing the tagline.
from PIL import Image, ImageDraw, ImageFont
W, H = 1200, 630
img = Image.new("RGB", (W, H), "#0f1117")
d = ImageDraw.Draw(img)
for y in range(H):  # subtle indigo glow
    a = max(0, 1 - y / 420)
    d.line([(0, y), (W, y)], fill=(int(15 + 64 * a * .5), int(17 + 70 * a * .35), int(23 + 229 * a * .45)))
bold = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
reg = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
# logo: stairs
d.rounded_rectangle([80, 80, 176, 176], 22, fill="#4f46e5")
d.line([(101, 152), (116, 152), (116, 140), (131, 140), (131, 128), (146, 128), (146, 116), (158, 116)], fill="white", width=7, joint="curve")
d.ellipse([150, 96, 166, 112], fill="#2dd4bf")
d.text((200, 100), "Zero to Prod", font=ImageFont.truetype(bold, 58), fill="white")
d.text((80, 250), "Learn backend, DevOps & AI engineering", font=ImageFont.truetype(bold, 50), fill="white")
d.text((80, 318), "from first principles.", font=ImageFont.truetype(bold, 50), fill="#818cf8")
d.text((80, 420), "Build and ship one real system. Interactive. Free forever.", font=ImageFont.truetype(reg, 32), fill="#cbd5e1")
d.text((80, 530), "jawwadzafar.github.io/zero-to-prod", font=ImageFont.truetype(reg, 28), fill="#2dd4bf")
img.save("static/img/social-card.png", optimize=True)
print("ok")
