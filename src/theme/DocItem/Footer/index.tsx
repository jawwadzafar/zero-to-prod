import React from 'react';
import Footer from '@theme-original/DocItem/Footer';
import type FooterType from '@theme/DocItem/Footer';
import type {WrapperProps} from '@docusaurus/types';
import {useDoc} from '@docusaurus/plugin-content-docs/client';
import ChapterComplete from '@site/src/components/ChapterComplete';

type Props = WrapperProps<typeof FooterType>;

export default function FooterWrapper(props: Props) {
  const {metadata} = useDoc();
  return (
    <>
      <ChapterComplete docId={metadata.id} />
      <Footer {...props} />
    </>
  );
}
