import { Prop, Schema, SchemaFactory } from '@nestjs/mongoose';
import { Document, Types } from 'mongoose';
import { ReleaseDate } from 'moviedb-promise';
// eslint-disable-next-line @typescript-eslint/no-var-requires
const paginate = require('mongoose-paginate-v2');

export type MovieDocument = Movie & Document;

@Schema()
export class Movie {
  @Prop({ type: Number, index: true, required: true })
  id: number;

  @Prop({ type: String })
  title: string;

  // @Prop({ type: Number, default: 0 })
  // budget: number;

  // @Prop({ type: Number, default: 0 })
  // revenue: number;

  @Prop({ type: String })
  original_title: string;

  @Prop({ type: String })
  poster_path: string;

  @Prop({ type: String })
  original_language: string;

  @Prop({ type: String })
  imdb_id: string;

  @Prop({ type: String })
  status: string;

  @Prop({ type: Number, default: null, index: -1 })
  popularity: number | null;

  @Prop({ type: Array, default: [] })
  genres: number[];

  @Prop({ type: Array, default: [] })
  production_companies: number[];

  @Prop({ type: Array, default: [] })
  release_date_th: ReleaseDate[];

  @Prop({ type: Number, default: null })
  collection_id: number | null;

  @Prop({ type: String, default: 'movie' })
  media_type: string;

  @Prop({
    required: false,
    index: true,
  })
  release_date: string;

  @Prop({ type: Number, nullable: true, required: false })
  runtime: number;

  // @Prop({ type: Boolean, default: false })
  // is_anime: boolean;

  @Prop({ type: Number, default: null })
  vote_average: number | null;

  @Prop({ type: Number, default: null })
  vote_count: number | null;

  @Prop({ type: Array, default: [] })
  watch_providers: [];

  @Prop({
    type: Date,
    default: new Date(),
    required: false,
  })
  updated_at: Date;
}

export const MovieSchema = SchemaFactory.createForClass(Movie);
MovieSchema.plugin(paginate);
MovieSchema.index({ id: 1 }, { unique: true });
MovieSchema.set('toJSON', {
  // transform: function (doc, ret) {
  //   delete ret._id;
  //   ret.id = Number(ret.id);
  //   delete ret.__v;
  // },
});
