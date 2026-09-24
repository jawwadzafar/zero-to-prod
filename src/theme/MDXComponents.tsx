import MDXComponents from '@theme-original/MDXComponents';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import Quiz from '@site/src/components/Quiz';
import SqlPlayground from '@site/src/components/SqlPlayground';
import Roadmap from '@site/src/components/Roadmap';
import Glossary from '@site/src/components/Glossary';

// Components usable in any .mdx chapter without an import line.
export default {
  ...MDXComponents,
  Tabs,
  TabItem,
  Quiz,
  SqlPlayground,
  Roadmap,
  Glossary,
};
