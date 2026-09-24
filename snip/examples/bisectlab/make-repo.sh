#!/usr/bin/env bash
# Builds a small practice Git repository for chapter 13.6's git bisect lab.
# Its history has 40 commits; one of them quietly breaks 3-character slugs.
# Your job: find which one, without reading them all.
#
#   ./make-repo.sh /tmp/bisect-demo      (default: ./bisect-demo)
#
# It only creates files inside that new directory. Delete it when done.
set -euo pipefail

dir=${1:-./bisect-demo}
if [ -e "$dir" ]; then
  echo "$dir already exists; pick another path or delete it first" >&2
  exit 1
fi
mkdir -p "$dir"
cd "$dir"
git init -q -b main
# A fixed, fake author so this works without your own Git identity configured.
export GIT_AUTHOR_NAME="Demo Dev" GIT_AUTHOR_EMAIL="demo@example.com"
export GIT_COMMITTER_NAME="Demo Dev" GIT_COMMITTER_EMAIL="demo@example.com"

min=3   # the minimum slug length; the bug changes this
reserved="api healthz readyz metrics"

write_validator() {
  cat > slug.sh <<SCRIPT
#!/usr/bin/env bash
# Usage: ./slug.sh SLUG  — exits 0 if SLUG is a valid short-link slug.
# Rules: letters, digits, - and _; length ${min} to 32; not a reserved word.
slug=\${1:-}
reserved="${reserved}"
for r in \$reserved; do
  if [ "\$slug" = "\$r" ]; then echo "reserved: \$slug"; exit 1; fi
done
if [[ "\$slug" =~ ^[A-Za-z0-9_-]{${min},32}\$ ]]; then
  echo "ok: \$slug"
else
  echo "invalid: \$slug"; exit 1
fi
SCRIPT
  chmod +x slug.sh
}

commit() { git add -A && git commit -q -m "$1"; }

write_validator
printf '# slugcheck\n\nValidates short-link slugs.\n' > README.md
printf '# Changelog\n' > CHANGELOG.md
commit "initial slug validator"

for i in $(seq 2 40); do
  case $i in
    27)
      min=4   # the regression: someone "tidied" the rules
      write_validator
      commit "slug: tidy validation rules"
      ;;
    9)
      reserved="$reserved docs"
      write_validator
      commit "slug: reserve 'docs'"
      ;;
    33)
      reserved="$reserved status"
      write_validator
      commit "slug: reserve 'status'"
      ;;
    *)
      if (( i % 3 == 0 )); then
        printf -- '- change %d\n' "$i" >> CHANGELOG.md
        commit "changelog: note change $i"
      else
        printf '\nNote %d: slugs are case-sensitive.\n' "$i" >> README.md
        commit "docs: README note $i"
      fi
      ;;
  esac
done

echo "Created $(git rev-list --count HEAD) commits in $dir"
echo "Try: cd $dir && ./slug.sh abc    (it should print 'ok: abc', but doesn't)"
