import Link from "next/link";
import { ArrowRight, ArrowUpRight } from "lucide-react";
import { PageLayout } from "@/components/layout/page-layout";
import { InspirationGallery } from "@/components/inspiration/gallery";
import { ReferenceCover } from "@/components/inspiration/reference-cover";
import type { CreationType } from "@/components/generation/creation-types";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { HomeCreation } from "./home-creation";

const destinations = [
  { title: "短剧工作室", href: "/short-drama", image: "alpine-road" },
  { title: "我的画布", href: "/assets?view=canvas", image: "moonlight-whale" },
  {
    title: "营销工作室",
    href: "/marketing-studio",
    image: "citrus-still-life",
  },
  { title: "动效工作室", href: "/motion-studio", image: "cobalt-headphones" },
];

export function HomePage({
  initialType = "image",
}: {
  initialType?: CreationType;
}) {
  return (
    <PageLayout screen="home" fullBleed>
      <div className="home-page">
        <HomeCreation initialType={initialType} />
        <section
          aria-labelledby="home-destinations"
          className="home-destinations"
        >
          <div className="workspace-section-heading">
            <h2 id="home-destinations">创作工作台</h2>
            <Button variant="ghost" asChild>
              <Link href="/ai-tools">
                所有工具{" "}
                <ArrowRight aria-hidden="true" data-icon="inline-end" />
              </Link>
            </Button>
          </div>
          <div className="home-destination-grid">
            <Card variant="feature" className="home-feature">
              <CardContent>
                <ReferenceCover image="amber-perfume" />
              </CardContent>
              <CardHeader>
                <CardTitle>探索创作灵感</CardTitle>
              </CardHeader>
              <Link
                href="/explore"
                className="home-feature-link"
                aria-label="探索创作灵感"
              >
                <ArrowUpRight aria-hidden="true" />
              </Link>
            </Card>
            <ul className="home-destination-links">
              {destinations.map(({ title, href, image }) => (
                <li key={href} className="min-w-0">
                  <Button
                    variant="secondary"
                    asChild
                    className="home-destination-link"
                  >
                    <Link href={href}>
                      <span className="home-destination-cover">
                        <ReferenceCover image={image} />
                      </span>
                      <span className="min-w-0 truncate">{title}</span>
                      <ArrowUpRight aria-hidden="true" data-icon="inline-end" />
                    </Link>
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        </section>
        <InspirationGallery />
      </div>
    </PageLayout>
  );
}
